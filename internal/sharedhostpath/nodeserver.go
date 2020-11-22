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
	"github.com/kazimsarikaya/csi-sharedhostpath/internal/volumepathhandler"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	klog "k8s.io/klog/v2"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
	"os"
	"time"
)

type nodeServer struct {
	nodeID            string
	maxVolumesPerNode int64
	caps              []*csi.NodeServiceCapability
	vh                *VolumeHelper
}

func NewNodeServer(nodeId string, maxVolumesPerNode int64, vh *VolumeHelper) *nodeServer {
	err := vh.UpdateNodeInfoLastSeen(nodeId, time.Now())
	if err != nil {
		klog.V(5).Infof("Cannot update node info %s %v", nodeId, err.Error())
	}
	lastSeenTicker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case t := <-lastSeenTicker.C:
				err := vh.UpdateNodeInfoLastSeen(nodeId, t)
				if err != nil {
					klog.V(5).Infof("Cannot update node info %s %v", nodeId, err.Error())
				} else {
					klog.V(5).Infof("update node info %s", nodeId)
				}
			}
		}
	}()

	return &nodeServer{
		nodeID:            nodeId,
		maxVolumesPerNode: maxVolumesPerNode,
		caps: []*csi.NodeServiceCapability{
			&csi.NodeServiceCapability{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
		vh: vh,
	}
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	topology := &csi.Topology{
		Segments: map[string]string{},
	}

	return &csi.NodeGetInfoResponse{
		NodeId:             ns.nodeID,
		MaxVolumesPerNode:  ns.maxVolumesPerNode,
		AccessibleTopology: topology,
	}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: ns.caps,
	}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	volumeId := req.GetVolumeId()
	if len(volumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume ID must be provided")
	}

	stagingTargetPath := req.GetStagingTargetPath()
	if len(stagingTargetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Staging Target Path must be provided")
	}

	cap := req.GetVolumeCapability()
	if cap == nil {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume Capability must be provided")
	}

	vol, err := ns.vh.GetVolume(volumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	mounter := mount.New("")

	if req.GetVolumeCapability().GetBlock() != nil {
		if !vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "cannot stage a non-block volume as block volume")
		}
		volPathHandler := volumepathhandler.VolumePathHandler{}

		// Get loop device from the volume path.
		loopDevice, err := volPathHandler.GetLoopDevice(vol.VolPath)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get the loop device: %v", err))
		}

		// Check if the target path exists. Create if not present.
		_, err = os.Lstat(stagingTargetPath)
		if os.IsNotExist(err) {
			var f *os.File
			f, err = os.OpenFile(stagingTargetPath, os.O_CREATE, 0640)
			if err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create target path: %s: %v", stagingTargetPath, err))
			}
			if err := f.Close(); err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create target path: %s: %v", stagingTargetPath, err))
			}
		}
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to check if the target block file exists: %v", err)
		}

		// Check if the target path is already mounted. Prevent remounting.
		notMount, err := mount.IsNotMountPoint(mounter, stagingTargetPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, status.Errorf(codes.Internal, "error checking path %s for mount: %s", stagingTargetPath, err)
			}
			notMount = true
		}
		if notMount {
			options := []string{"bind"}
			if err := mounter.Mount(loopDevice, stagingTargetPath, "", options); err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("failed to mount block device: %s at %s: %v", loopDevice, stagingTargetPath, err))
			}
		}
	} else if req.GetVolumeCapability().GetMount() != nil {
		volume_context := req.GetVolumeContext()
		var vtype string
		var found bool
		if vtype, found = volume_context[typeParameter]; !found {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("required parameter not found: %s", typeParameter))
		}
		if vtype == "disk" && !vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "cannot stage a non-block volume as disk volume")
		}
		if vtype == "folder" && vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "cannot stage a block volume as folder volume")
		}

		notMnt, err := mount.IsNotMountPoint(mounter, stagingTargetPath)
		if err != nil {
			if os.IsNotExist(err) {
				if err = os.MkdirAll(stagingTargetPath, 0750); err != nil {
					return nil, status.Error(codes.Internal, err.Error())
				}
				notMnt = true
			} else {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}

		if notMnt {
			options := []string{}

			if vtype == "folder" {
				options = append(options, "bind")
				if err := mounter.Mount(vol.VolPath, stagingTargetPath, "", options); err != nil {
					return nil, status.Error(codes.Internal, fmt.Sprintf("failed to mount device: %s at %s: %s", vol.VolPath, stagingTargetPath, err.Error()))
				}
			} else if vtype == "disk" {
				var fsType string
				if fsType, found = volume_context[fstypeParameter]; !found {
					return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("required parameter not found: %s", fstypeParameter))
				}
				if fsType == "xfs" {
					options = append(options, "nouuid")
				}
				volPathHandler := volumepathhandler.NewBlockVolumePathHandler()
				loopDevice, err := volPathHandler.AttachFileDevice(vol.VolPath)
				if err != nil {
					return nil, status.Error(codes.Internal, fmt.Sprintf("cannot create loop device: %s", err.Error()))
				}
				formatAndMount := mount.SafeFormatAndMount{Interface: mounter, Exec: utilexec.New()}
				err = formatAndMount.FormatAndMount(loopDevice, stagingTargetPath, fsType, options)
				if err != nil {
					return nil, status.Error(codes.Internal, fmt.Sprintf("failed to mount device: %s at %s: %s", vol.VolPath, stagingTargetPath, err.Error()))
				}
			} else {
				return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid volume type: %s", vtype))
			}
		}

	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	volumeId := req.GetVolumeId()
	if len(volumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume ID must be provided")
	}

	stagingTargetPath := req.GetStagingTargetPath()
	if len(stagingTargetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Staging Target Path must be provided")
	}

	vol, err := ns.vh.GetVolume(volumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	if notMnt, err := mount.IsNotMountPoint(mount.New(""), stagingTargetPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if !notMnt {
		err = mount.New("").Unmount(stagingTargetPath)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if vol.IsBlock {
		volPathHandler := volumepathhandler.NewBlockVolumePathHandler()
		err := volPathHandler.DetachFileDevice(vol.VolPath)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if err = os.RemoveAll(stagingTargetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(5).Infof("hostpath: volume %s has been unstaged.", stagingTargetPath)

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	cap := req.GetVolumeCapability()
	if cap == nil {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume capability missing in request")
	}

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume ID missing in request")
	}

	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Target path missing in request")
	}

	stagingTargetPath := req.GetStagingTargetPath()
	if len(stagingTargetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Staging Target Path must be provided")
	}

	vol, err := ns.vh.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	if cap.GetBlock() != nil {
		if !vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "cannot publish a non-block volume as block volume")
		}
	}

	mounter := mount.New("")

	notMnt, err := mount.IsNotMountPoint(mounter, targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if notMnt {
		options := []string{"bind"}

		readOnly := req.GetReadonly()
		if readOnly {
			options = append(options, "ro")
		}

		if err := mounter.Mount(stagingTargetPath, targetPath, "", options); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to mount block device: %s at %s: %v", stagingTargetPath, targetPath, err))
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}
	targetPath := req.GetTargetPath()
	volumeID := req.GetVolumeId()

	_, err := ns.vh.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	if notMnt, err := mount.IsNotMountPoint(mount.New(""), targetPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if !notMnt {
		err = mount.New("").Unmount(targetPath)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if err = os.RemoveAll(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(5).Infof("hostpath: volume %s has been unpublished.", targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

/* Unimplemented methods beyond */

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, in *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
