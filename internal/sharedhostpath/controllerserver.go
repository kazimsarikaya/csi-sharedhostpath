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
)

type controllerServer struct {
	caps   []*csi.ControllerServiceCapability
	nodeID string
	vh     *VolumeHelper
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

	capacity := uint64(req.GetCapacityRange().GetRequiredBytes())
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

	nvpi, err := cs.vh.GetNodePublishVolumeInfo(volumeID, nodeID)
	if err == nil {
		if nvpi != nil {
			if nvpi.ReadOnly != req.Readonly {
				return nil, status.Error(codes.AlreadyExists, "cannot publish readonly status dismatch")
			} else {
				return &csi.ControllerPublishVolumeResponse{
					PublishContext: map[string]string{},
				}, nil
			}
		}
	} else {
		if nvpi != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("error at getting nvpi vol %s node %s: %v", volumeID, nodeID, err))
		}
	}

	err = cs.vh.CreateNodePublishVolumeInfo(volumeID, nodeID, req.Readonly)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("cannot create nvpi vol %s node %s: %v", volumeID, nodeID, err))
	}

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{},
	}, nil
}

func (cs *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume ID must be provided")
	}

	volumeID := req.GetVolumeId()

	_, err := cs.vh.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("volume %s not found: %v", volumeID, err))
	}

	nodeID := req.GetNodeId()
	nvpi, err := cs.vh.GetNodePublishVolumeInfo(volumeID, nodeID)

	if err == nil {
		err = cs.vh.DeleteNodePublishVolumeInfo(volumeID, nodeID)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("error at deleting nvpi vol %s node %s: %v", volumeID, nodeID, err))
		}
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	} else {
		if nvpi != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("error at getting nvpi vol %s node %s: %v", volumeID, nodeID, err))
		} else {
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
	}
}

/* Unimplemented methods beyond */

func (cs *controllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
