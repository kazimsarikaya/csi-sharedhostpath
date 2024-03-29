/*
Copyright 2020 Kazım SARIKAYA

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sharedhostpath

import (
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	klog "k8s.io/klog/v2"
	"math"
	"strconv"
	"sync"
)

type controllerServer struct {
	caps   []*csi.ControllerServiceCapability
	nodeID string
	vh     *VolumeHelper
	mutex  sync.Mutex
}

const (
	pvcNameKey      = "csi.storage.k8s.io/pvc/name"
	pvcNamespaceKey = "csi.storage.k8s.io/pvc/namespace"
	pvNameKey       = "csi.storage.k8s.io/pv/name"
)

const (
	deviceID           = "deviceID"
	maxStorageCapacity = 1 << 40
)

func NewControllerServer(nodeID string, vh *VolumeHelper) *controllerServer {
	return &controllerServer{
		caps: getControllerServiceCapabilities(
			[]csi.ControllerServiceCapability_RPC_Type{
				csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
				csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
				csi.ControllerServiceCapability_RPC_PUBLISH_READONLY,
				csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
				csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
				csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES,
				csi.ControllerServiceCapability_RPC_GET_VOLUME,
				csi.ControllerServiceCapability_RPC_VOLUME_CONDITION,
			}),
		nodeID: nodeID,
		vh:     vh,
	}
}

func (cs *controllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.caps,
	}, nil
}

func getControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) []*csi.ControllerServiceCapability {
	var csc []*csi.ControllerServiceCapability

	for _, cap := range cl {
		klog.V(5).Infof("Enabling controller service capability: %v", cap.String())
		csc = append(csc, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		})
	}

	return csc
}

func (cs *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	// Check arguments
	var vol *Volume
	var err error

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}
	if len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, req.VolumeId)
	}

	if vol, err = cs.vh.GetVolume(req.GetVolumeId()); err != nil {
		return nil, status.Error(codes.NotFound, req.GetVolumeId())
	}

	for _, cap := range req.GetVolumeCapabilities() {
		if cap.GetMount() == nil && cap.GetBlock() == nil {
			return nil, status.Error(codes.InvalidArgument, "cannot have both mount and block access type be undefined")
		}
		if vol.IsBlock && cap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return nil, status.Error(codes.InvalidArgument, "block backend can be accessd only SINGLE_NODE_WRITER")
		}
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeContext:      req.GetVolumeContext(),
			VolumeCapabilities: req.GetVolumeCapabilities(),
			Parameters:         req.GetParameters(),
		},
	}, nil
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Name missing in request")
	}
	caps := req.GetVolumeCapabilities()
	if caps == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities missing in request")
	}

	var accessTypeMount, accessTypeBlock, isBlock bool

	for _, cap := range caps {
		if cap.GetBlock() != nil {
			accessTypeBlock = true
			isBlock = true
		}
		if cap.GetMount() != nil {
			accessTypeMount = true
			isBlock = false
		}
	}

	if accessTypeBlock && accessTypeMount {
		return nil, status.Error(codes.InvalidArgument, "cannot have both block and mount access type")
	}

	parameters := req.GetParameters()
	var vtype string
	var found bool
	if vtype, found = parameters[typeParameter]; !found {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("storage class parameter required: %s", typeParameter))
	}
	if vtype == "disk" {
		isBlock = true
	}
	if vtype == "folder" && accessTypeBlock {
		return nil, status.Error(codes.InvalidArgument, "cannot have both folder type and block access type")
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	capacity = fixCapacity(capacity)
	if capacity >= maxStorageCapacity {
		return nil, status.Errorf(codes.OutOfRange, "Requested capacity %d exceeds maximum allowed %d", capacity, maxStorageCapacity)
	}

	if isBlock {
		for _, cap := range caps {
			if cap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
				return nil, status.Error(codes.InvalidArgument, "block backend can be accessd only SINGLE_NODE_WRITER")
			}
		}
	}

	volName := req.GetName()

	topologies := []*csi.Topology{&csi.Topology{
		Segments: map[string]string{},
	}}

	if volid, err := cs.vh.GetVolumeIdByName(volName); err == nil {
		vol, err := cs.vh.GetVolume(volid)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("cannot get volume status: %v", err.Error()))
		}
		preq, err := vol.PopulateVolumeIfRequired()
		if err == nil {
			if preq || (vol.Capacity == capacity && vol.IsBlock == isBlock) {
				return &csi.CreateVolumeResponse{
					Volume: &csi.Volume{
						VolumeId:           vol.VolID,
						CapacityBytes:      req.GetCapacityRange().GetRequiredBytes(),
						VolumeContext:      req.GetParameters(),
						ContentSource:      req.GetVolumeContentSource(),
						AccessibleTopology: topologies,
					},
				}, nil
			} else {
				return nil, status.Error(codes.AlreadyExists, "Volume already exists")
			}
		} else {
			return nil, status.Error(codes.Internal, fmt.Sprintf("cannot check volume status: %v", err.Error()))
		}
	}

	params := req.GetParameters()

	nsName, found := params[pvcNamespaceKey]
	if !found {
		return nil, status.Error(codes.InvalidArgument, "Namespace name parameter missing in request")
	}

	pvName, found := params[pvNameKey]
	if !found {
		return nil, status.Error(codes.InvalidArgument, "PV name parameter missing in request")
	}

	pvcName, found := params[pvcNameKey]
	if !found {
		return nil, status.Error(codes.InvalidArgument, "PVC name parameter missing in request")
	}

	r_uuid, err := uuid.NewRandom()
	if err != nil {
		return nil, status.Error(codes.Internal, "cannot generate volume id")
	}

	volumeID := r_uuid.String()

	vol, err := cs.vh.CreateVolume(volumeID, volName, pvName, pvcName, nsName, capacity, isBlock)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create volume %v: %v", volumeID, err)
	}
	klog.V(5).Infof("created volume %s at path %s", vol.VolID, vol.VolPath)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:           vol.VolID,
			CapacityBytes:      req.GetCapacityRange().GetRequiredBytes(),
			VolumeContext:      req.GetParameters(),
			ContentSource:      req.GetVolumeContentSource(),
			AccessibleTopology: topologies,
		},
	}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	volId := req.GetVolumeId()

	vol, err := cs.vh.GetVolume(volId)
	if err != nil {
		status.Errorf(codes.Internal, "failed to get volume %v: %v", volId, err)
	}

	if vol == nil {
		return &csi.DeleteVolumeResponse{}, nil
	}

	if err := cs.vh.DeleteVolume(volId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete volume %v: %v", volId, err)
	}

	klog.V(5).Infof("volume %v successfully deleted", volId)

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume ID must be provided")
	}

	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Node ID must be provided")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume capability must be provided")
	}

	volumeID := req.GetVolumeId()

	_, err := cs.vh.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("volume %s not found: %v", volumeID, err))
	}

	nodeID := req.GetNodeId()
	ni, err := cs.vh.GetNodeInfo(nodeID, 30*1000)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("eror at checking node %s: %v", nodeID, err))
	} else {
		if ni == nil {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("node %s not found: %v", nodeID, err))
		}
	}

	cpvi, err := cs.vh.GetControllerPublishVolumeInfo(volumeID, nodeID)
	if err == nil {
		if cpvi != nil {
			if cpvi.ReadOnly != req.Readonly {
				return nil, status.Error(codes.AlreadyExists, "cannot publish readonly status dismatch")
			} else {
				return &csi.ControllerPublishVolumeResponse{
					PublishContext: map[string]string{},
				}, nil
			}
		}
	} else {
		if cpvi != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("error at getting cpvi vol %s node %s: %v", volumeID, nodeID, err))
		}
	}

	err = cs.vh.CreateControllerPublishVolumeInfo(volumeID, nodeID, req.Readonly)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("cannot create cpvi vol %s node %s: %v", volumeID, nodeID, err))
	}

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{},
	}, nil
}

func (cs *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume ID must be provided")
	}

	volumeID := req.GetVolumeId()

	_, err := cs.vh.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("volume %s not found: %v", volumeID, err))
	}

	nodeID := req.GetNodeId()
	cpvi, err := cs.vh.GetControllerPublishVolumeInfo(volumeID, nodeID)

	if err == nil {
		err = cs.vh.DeleteControllerPublishVolumeInfo(volumeID, nodeID)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("error at deleting cpvi vol %s node %s: %v", volumeID, nodeID, err))
		}
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	} else {
		if cpvi != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("error at getting cpvi vol %s node %s: %v", volumeID, nodeID, err))
		} else {
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
	}
}

func (cs *controllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerExpandVolume volume ID missing in request")
	}

	capRange := req.GetCapacityRange()
	if capRange == nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerExpandVolume capacity range missing in request")
	}

	newCapacity := int64(capRange.GetRequiredBytes())
	newCapacity = fixCapacity(newCapacity)
	if newCapacity >= maxStorageCapacity {
		return nil, status.Errorf(codes.OutOfRange, "Requested capacity %d exceeds maximum allowed %d", newCapacity, maxStorageCapacity)
	}

	vol, err := cs.vh.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("volume %s not found: %v", volumeID, err))
	}

	if newCapacity <= vol.Capacity {
		return &csi.ControllerExpandVolumeResponse{CapacityBytes: vol.Capacity, NodeExpansionRequired: false}, nil
	}

	if err := cs.vh.UpdateVolumeCapacity(vol, newCapacity); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("ControllerExpandVolume cannot update volume size on db: %v", err.Error()))
	}

	var need_node_expand bool = false
	if vol.IsBlock {
		need_node_expand = true
	}

	return &csi.ControllerExpandVolumeResponse{CapacityBytes: newCapacity, NodeExpansionRequired: need_node_expand}, nil
}

func (cs *controllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	var startingToken int = 0
	if req.StartingToken != "" {
		parsedToken, err := strconv.ParseInt(req.StartingToken, 10, 32)
		if err != nil {
			return nil, status.Errorf(codes.Aborted, "ListVolumes starting token %q is not valid: %s", req.StartingToken, err)
		}
		startingToken = int(parsedToken)
	}

	maxEntries := int(req.MaxEntries)
	if maxEntries == 0 {
		maxEntries = math.MaxInt32
	}

	vols, err := cs.vh.GetVolumesWithDetail(startingToken, maxEntries)

	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("ListVolumes cannot get volume list from db: %v", err.Error()))
	}

	var entries []*csi.ListVolumesResponse_Entry

	for _, vol := range vols {
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      vol["volumeId"].(string),
				CapacityBytes: vol["capacity"].(int64),
				VolumeContext: vol["parameters"].(map[string]string),
			},
			Status: &csi.ListVolumesResponse_VolumeStatus{
				PublishedNodeIds: vol["published_node_ids"].([]string),
				VolumeCondition: &csi.VolumeCondition{
					Abnormal: vol["condition_abnormal"].(bool),
					Message:  vol["condition_msg"].(string),
				},
			},
		})
	}

	nextStartingToken := -1
	vc, err := cs.vh.GetVolumeCount()
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("ListVolumes cannot get volume count from db: %v", err.Error()))
	}

	if maxEntries != math.MaxInt32 {
		nextStartingToken = startingToken + maxEntries
		if nextStartingToken >= vc {
			nextStartingToken = -1
		}
	}

	if nextStartingToken != -1 {

		return &csi.ListVolumesResponse{
			Entries:   entries,
			NextToken: strconv.FormatInt(int64(nextStartingToken), 10),
		}, nil
	}
	return &csi.ListVolumesResponse{
		Entries: entries,
	}, nil
}

func (cs *controllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerGetVolume volume ID missing in request")
	}

	vol, err := cs.vh.GetVolumeWithDetail(volumeID)

	if err != nil {
		errstr := fmt.Sprintf("ControllerGetVolume error while getting volume detail: %v", err.Error())
		klog.Errorf(errstr)
		return nil, status.Error(codes.Internal, errstr)
	}

	if vol == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("ControllerGetVolume volume %s not found", volumeID))
	}

	return &csi.ControllerGetVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vol["volumeId"].(string),
			CapacityBytes: vol["capacity"].(int64),
			VolumeContext: vol["parameters"].(map[string]string),
		},
		Status: &csi.ControllerGetVolumeResponse_VolumeStatus{
			PublishedNodeIds: vol["published_node_ids"].([]string),
			VolumeCondition: &csi.VolumeCondition{
				Abnormal: vol["condition_abnormal"].(bool),
				Message:  vol["condition_msg"].(string),
			},
		},
	}, nil
}

/* Unimplemented methods beyond */

func (cs *controllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
