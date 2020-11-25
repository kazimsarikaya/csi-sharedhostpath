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
	"github.com/kazimsarikaya/csi-sharedhostpath/internal/volumehelpers"
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
		klog.V(4).Error(err, "Cannot update node info %s", nodeId)
	}
	lastSeenTicker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case t := <-lastSeenTicker.C:
				err := vh.UpdateNodeInfoLastSeen(nodeId, t)
				if err != nil {
					klog.V(4).Infof("Cannot update node info %s %v", nodeId, err.Error())
				} else {
					klog.V(4).Infof("update node info %s", nodeId)
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
			&csi.NodeServiceCapability{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
			&csi.NodeServiceCapability{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
			&csi.NodeServiceCapability{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_VOLUME_CONDITION,
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

	klog.V(2).Infof("NodeStageVolume volume %s will be staged to the path %s", volumeId, stagingTargetPath)

	mounter := mount.New("")

	if req.GetVolumeCapability().GetBlock() != nil {
		klog.V(4).Infof("NodeStageVolume volume %s will be staged to the path %s as raw", volumeId, stagingTargetPath)
		if !vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "NodeStageVolume cannot stage a non-block volume as block volume")
		}
		volumePathHandler := volumehelpers.VolumePathHandler{}

		// Get loop device from the volume path.
		loopDevice, err := volumePathHandler.AttachFileDevice(vol.VolPath)
		if err != nil {
			klog.V(4).Error(err, "")
			return nil, status.Error(codes.Internal, fmt.Sprintf("NodeStageVolume failed to get the loop device: %v", err))
		}
		klog.V(4).Infof("NodeStageVolume volume %s attached to the device %s", volumeId, loopDevice)

		// Check if the target path exists. Create if not present.
		_, err = os.Lstat(stagingTargetPath)
		if os.IsNotExist(err) {
			var f *os.File
			f, err = os.OpenFile(stagingTargetPath, os.O_CREATE, 0640)
			if err != nil {
				klog.V(4).Error(err, "NodeStageVolume failed to create target path %s for volume %s", stagingTargetPath, volumeId)
				return nil, status.Error(codes.Internal, fmt.Sprintf("NodeStageVolume failed to create target path: %s: %v", stagingTargetPath, err))
			}
			if err := f.Close(); err != nil {
				klog.V(4).Error(err, "NodeStageVolume failed to create target path %s for volume %s", stagingTargetPath, volumeId)
				return nil, status.Error(codes.Internal, fmt.Sprintf("NodeStageVolume failed to create target path: %s: %v", stagingTargetPath, err))
			}
		}
		if err != nil {
			klog.V(4).Error(err, "NodeStageVolume failed to check if the target block file %s exists for volume %s", stagingTargetPath, volumeId)
			return nil, status.Errorf(codes.Internal, "NodeStageVolume failed to check if the target block file exists: %v", err)
		}

		// Check if the target path is already mounted. Prevent remounting.
		notMount, err := mount.IsNotMountPoint(mounter, stagingTargetPath)
		if err != nil {
			if !os.IsNotExist(err) {
				klog.V(4).Error(err, "NodeStageVolume failed to check mount status of path %s for volume %s", stagingTargetPath, volumeId)
				return nil, status.Errorf(codes.Internal, "NodeStageVolume error checking path %s for mount: %s", stagingTargetPath, err)
			}
			notMount = true
		}
		if notMount {
			options := []string{"bind"}
			if err := mounter.Mount(loopDevice, stagingTargetPath, "", options); err != nil {
				klog.V(4).Error(err, "NodeStageVolume failed to mount loop device %s to the staging target %s for volume %s", loopDevice, stagingTargetPath, volumeId)
				return nil, status.Error(codes.Internal, fmt.Sprintf("NodeStageVolume failed to mount block device: %s at %s: %v", loopDevice, stagingTargetPath, err))
			}
		}
		klog.V(4).Infof("NodeStageVolume volume %s staged to the path %s from loop device %s as raw", volumeId, stagingTargetPath, loopDevice)
	} else if req.GetVolumeCapability().GetMount() != nil {
		volume_context := req.GetVolumeContext()
		var vtype string
		var found bool
		if vtype, found = volume_context[typeParameter]; !found {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("NodeStageVolume required parameter not found: %s", typeParameter))
		}
		if vtype == "disk" && !vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "NodeStageVolume cannot stage a non-block volume as disk volume")
		}
		if vtype == "folder" && vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "NodeStageVolume cannot stage a block volume as folder volume")
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
					return nil, status.Error(codes.Internal, fmt.Sprintf("NodeStageVolume failed to mount device: %s at %s: %s", vol.VolPath, stagingTargetPath, err.Error()))
				}
			} else if vtype == "disk" {
				var fsType string
				if fsType, found = volume_context[fstypeParameter]; !found {
					return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("NodeStageVolume required parameter not found: %s", fstypeParameter))
				}
				if fsType == "xfs" {
					options = append(options, "nouuid")
				}
				volumePathHandler := volumehelpers.NewBlockVolumePathHandler()
				loopDevice, err := volumePathHandler.AttachFileDevice(vol.VolPath)
				if err != nil {
					return nil, status.Error(codes.Internal, fmt.Sprintf("NodeStageVolume cannot create loop device: %s", err.Error()))
				}
				formatAndMount := mount.SafeFormatAndMount{Interface: mounter, Exec: utilexec.New()}
				err = formatAndMount.FormatAndMount(loopDevice, stagingTargetPath, fsType, options)
				if err != nil {
					return nil, status.Error(codes.Internal, fmt.Sprintf("NodeStageVolume failed to mount device: %s at %s: %s", vol.VolPath, stagingTargetPath, err.Error()))
				}
			} else {
				return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("NodeStageVolume invalid volume type: %s", vtype))
			}
		}

	}

	klog.V(2).Infof("NodeStageVolume volume %s staged to the path %s", volumeId, stagingTargetPath)

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

	klog.V(2).Infof("NodeUnstageVolume try to unmount volume %s at %s", volumeId, stagingTargetPath)

	mounter := mount.New("")
	if notMnt, err := mount.IsNotMountPoint(mounter, stagingTargetPath); err != nil {
		if !os.IsNotExist(err) {
			klog.V(4).Error(err, "NodeUnstageVolume an error occured at checking volume %s  mount point %s", volumeId, stagingTargetPath)
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if !notMnt {
		err = mounter.Unmount(stagingTargetPath)
		if err != nil {
			klog.V(4).Error(err, "NodeUnstageVolume an error occured at unmounting volume %s at mount point %s: %v", volumeId, stagingTargetPath)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	klog.V(4).Infof("NodeUnstageVolume unmount volume %s at %s succeeded", volumeId, stagingTargetPath)

	if vol.IsBlock {
		klog.V(4).Infof("NodeUnstageVolume try detach volume %s device %s", volumeId, vol.VolPath)
		volumeHelpers := volumehelpers.NewBlockVolumePathHandler()
		err := volumeHelpers.DetachFileDevice(vol.VolPath)
		if err != nil {
			klog.V(4).Error(err, "NodeUnstageVolume detach failed volume %s device %s", volumeId, vol.VolPath)
			return nil, status.Error(codes.Internal, err.Error())
		}
		klog.V(4).Infof("NodeUnstageVolume detach volume %s device %s succeeded", volumeId, vol.VolPath)
	}

	klog.V(4).Infof("NodeUnstageVolume try to remove mount path %s for volume %s", stagingTargetPath, volumeId)
	if err = os.RemoveAll(stagingTargetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Infof("NodeUnstageVolume volume %s has been unstaged.", stagingTargetPath)

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

	var rawMount bool = false
	if cap.GetBlock() != nil {
		if !vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "cannot publish a non-block volume as block volume")
		}
		rawMount = true
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

	readOnly := req.GetReadonly()
	if notMnt {
		options := []string{"bind"}
		if readOnly {
			options = append(options, "ro")
		}

		if err := mounter.Mount(stagingTargetPath, targetPath, "", options); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to mount block device: %s at %s: %v", stagingTargetPath, targetPath, err))
		}
	}

	err = ns.vh.CreateNodePublishVolumeInfo(ns.nodeID, volumeID, targetPath, rawMount, readOnly)
	if err != nil { // TODO: how to be impodent
		mount.New("").Unmount(targetPath)
		os.RemoveAll(targetPath)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to update node %s volume %s path %s info: %v", ns.nodeID, volumeID, targetPath, err))
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

	err = ns.vh.DeleteNodePublishVolumeInfo(ns.nodeID, volumeID, targetPath)
	if err != nil {
		klog.Errorf("cannot delete node %s publish volume %s info at path %s: %v", ns.nodeID, volumeID, targetPath, err)
	}
	klog.V(5).Infof("hostpath: volume %s has been unpublished.", targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeExpandVolume volume ID not provided")
	}

	volumePath := req.GetVolumePath()
	if len(volumePath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeExpandVolume volume path not provided")
	}

	vol, err := ns.vh.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	if !vol.IsBlock {
		return &csi.NodeExpandVolumeResponse{}, nil
	}

	resize_fs := true
	cap := req.GetVolumeCapability()
	if cap != nil {
		if cap.GetBlock() != nil {
			resize_fs = false
		}
	}

	volumePathHandler := volumehelpers.VolumePathHandler{}
	loopDevice, err := volumePathHandler.GetLoopDevice(vol.VolPath)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get the loop device: %v", err))
	}

	err = volumePathHandler.ReReadFileSize(vol.VolPath)

	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("cannot resize backend: %v", err.Error()))
	}

	if resize_fs {
		mounter := mount.New("")
		notMnt, err := mount.IsNotMountPoint(mounter, volumePath)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "NodeExpandVolume failed to check if volume path %q is mounted: %s", volumePath, err)
		}

		if notMnt {
			return nil, status.Errorf(codes.NotFound, "NodeExpandVolume volume path %q is not mounted", volumePath)
		}

		r := volumehelpers.NewResizeFs(&mount.SafeFormatAndMount{Interface: mounter, Exec: utilexec.New()})
		klog.Infof("NodeExpandVolume Try to expand volume %s at %s", volumeID, volumePath)
		if _, err := r.Resize(loopDevice, volumePath); err != nil {
			return nil, status.Errorf(codes.Internal, "NodeExpandVolume could not resize volume %q (%q):  %v", volumeID, req.GetVolumePath(), err)
		} else {
			klog.Infof("NodeExpandVolume Volume %s at %s expanded", volumeID, volumePath)
		}
	}

	return &csi.NodeExpandVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume ID not provided")
	}

	volumePath := req.GetVolumePath()
	if len(volumePath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume path not provided")
	}

	_, err := ns.vh.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	_, err = ns.vh.GetNodePublishVolumeInfo(ns.nodeID, volumeID, volumePath)

	if err != nil {
		return nil, status.Errorf(codes.NotFound, "NodeGetVolumeStats volumePath %s is not same as volume's published mount", volumePath)
	}

	fi, err := os.Stat(volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "NodeGetVolumeStats cannot stat volumepath: %s : %v", volumePath, err)
	}

	var usage []*csi.VolumeUsage
	var condition = &csi.VolumeCondition{}
	if (fi.Mode() & os.ModeDir) == os.ModeDir {
		stats, err := getStatistics(volumePath)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "NodeGetVolumeStats failed to retrieve capacity statistics for volume path %q: %s", volumePath, err)
		}
		usage = []*csi.VolumeUsage{
			&csi.VolumeUsage{
				Available: stats.availableBytes,
				Total:     stats.totalBytes,
				Used:      stats.usedBytes,
				Unit:      csi.VolumeUsage_BYTES,
			},
			&csi.VolumeUsage{
				Available: stats.availableInodes,
				Total:     stats.totalInodes,
				Used:      stats.usedInodes,
				Unit:      csi.VolumeUsage_INODES,
			},
		}
		//TODO: extra check for condition
		condition.Abnormal = false
		condition.Message = "ok"
	} else {
		totalBytes, err := getBlockDeviceSize(volumePath)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "NodeGetVolumeStats cannot get devicesize: %v", err)
		}
		usage = []*csi.VolumeUsage{
			&csi.VolumeUsage{
				Unit:  csi.VolumeUsage_BYTES,
				Total: totalBytes,
			},
		}
		//TODO: extra check for condition
		condition.Abnormal = false
		condition.Message = "ok"
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage:           usage,
		VolumeCondition: condition,
	}, nil
}
