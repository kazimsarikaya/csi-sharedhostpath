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
	"errors"
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

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	cap := req.GetVolumeCapability()
	if cap == nil {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume capability missing in request")
	}

	volumeId := req.GetVolumeId()
	if len(volumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume ID missing in request")
	}

	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Target path missing in request")
	}

	klog.V(4).Infof("NodePublishVolume try to mount volume %s on node %s for path %s", volumeId, ns.nodeID, targetPath)

	vol, err := ns.vh.GetVolume(volumeId)
	if err != nil {
		klog.V(4).Error(err, fmt.Sprintf("NodePublishVolume cannot find volume %s on node %s for path %s", volumeId, ns.nodeID, targetPath))
		return nil, status.Error(codes.NotFound, err.Error())
	}

	mounter := mount.New("")

	options := []string{}

	readOnly := req.GetReadonly()
	if readOnly {
		klog.V(4).Infof("NodePublishVolume readonly mount volume %s on node %s for path %s", volumeId, ns.nodeID, targetPath)
		options = append(options, "ro")
	}

	rawMount := false

	if req.GetVolumeCapability().GetBlock() != nil {
		klog.V(4).Infof("NodePublishVolume volume %s will be mounted to the path %s as raw", volumeId, targetPath)
		if !vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "NodePublishVolume cannot mount a non-block volume as block volume")
		}
		volumePathHandler := volumehelpers.VolumePathHandler{}

		// Get loop device from the volume path.
		loopDevice, err := volumePathHandler.AttachFileDevice(vol.VolPath)
		if err != nil {
			klog.V(4).Error(err, "")
			return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume failed to get the loop device: %v", err))
		}
		klog.V(4).Infof("NodePublishVolume volume %s attached to the device %s", volumeId, loopDevice)

		// Check if the target path exists. Create if not present.
		_, err = os.Lstat(targetPath)
		if os.IsNotExist(err) {
			var f *os.File
			f, err = os.OpenFile(targetPath, os.O_CREATE, 0777)
			if err != nil {
				klog.V(4).Error(err, "NodePublishVolume failed to create target path %s for volume %s", targetPath, volumeId)
				return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume failed to create target path: %s: %v", targetPath, err))
			}
			if err := f.Close(); err != nil {
				klog.V(4).Error(err, "NodePublishVolume failed to create target path %s for volume %s", targetPath, volumeId)
				return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume failed to create target path: %s: %v", targetPath, err))
			}
		}
		if err != nil {
			klog.V(4).Error(err, "NodePublishVolume failed to check if the target block file %s exists for volume %s", targetPath, volumeId)
			return nil, status.Errorf(codes.Internal, "NodePublishVolume failed to check if the target block file exists: %v", err)
		}

		// Check if the target path is already mounted. Prevent remounting.
		notMount, err := mount.IsNotMountPoint(mounter, targetPath)
		if err != nil {
			if !os.IsNotExist(err) {
				klog.V(4).Error(err, "NodePublishVolume failed to check mount status of path %s for volume %s", targetPath, volumeId)
				return nil, status.Errorf(codes.Internal, "NodePublishVolume error checking path %s for mount: %s", targetPath, err)
			}
			notMount = true
		}
		if notMount {
			options = append(options, "bind")
			if err := mounter.Mount(loopDevice, targetPath, "", options); err != nil {
				klog.V(4).Error(err, "NodePublishVolume failed to mount loop device %s to the target %s for volume %s", loopDevice, targetPath, volumeId)
				return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume failed to mount block device: %s at %s: %v", loopDevice, targetPath, err))
			}
		}
		rawMount = true
		klog.V(4).Infof("NodePublishVolume volume %s mounted to the path %s from loop device %s as raw", volumeId, targetPath, loopDevice)
	} else if req.GetVolumeCapability().GetMount() != nil {
		volume_context := req.GetVolumeContext()
		var vtype string
		var found bool
		if vtype, found = volume_context[typeParameter]; !found {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("NodePublishVolume required parameter not found: %s", typeParameter))
		}
		if vtype == "disk" && !vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "NodePublishVolume cannot mount a non-block volume as disk volume")
		}
		if vtype == "folder" && vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "NodePublishVolume cannot mount a block volume as folder volume")
		}

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
			if vtype == "folder" {
				options = append(options, "bind")
				if err := mounter.Mount(vol.VolPath, targetPath, "", options); err != nil {
					klog.V(4).Error(err, fmt.Sprintf("NodePublishVolume failed to mount volume %s to %s on node %s", vol.VolPath, targetPath, ns.nodeID))
					return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume failed to mount device: %s at %s: %s", vol.VolPath, targetPath, err.Error()))
				}
			} else if vtype == "disk" {
				var fsType string
				if fsType, found = volume_context[fstypeParameter]; !found {
					return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("NodePublishVolume required parameter not found: %s", fstypeParameter))
				}
				if fsType == "xfs" {
					options = append(options, "nouuid")
				}
				volumePathHandler := volumehelpers.NewBlockVolumePathHandler()
				loopDevice, err := volumePathHandler.AttachFileDevice(vol.VolPath)
				if err != nil {
					klog.V(4).Error(err, fmt.Sprintf("NodePublishVolume cannot create loop device: %s for volume %s on node %s", loopDevice, volumeId, ns.nodeID))
					return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume cannot create loop device: %s", err.Error()))
				}
				formatAndMount := mount.SafeFormatAndMount{Interface: mounter, Exec: utilexec.New()}
				err = formatAndMount.FormatAndMount(loopDevice, targetPath, fsType, options)
				if err != nil {
					klog.V(4).Error(err, fmt.Sprintf("NodePublishVolume failed to mount device: %s to %s on node %s", loopDevice, targetPath, ns.nodeID))
					return nil, status.Error(codes.Internal, fmt.Sprintf("NodePublishVolume failed to mount device: %s at %s: %s", loopDevice, targetPath, err.Error()))
				}
			} else {
				return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("NodePublishVolume invalid volume type: %s", vtype))
			}
		}
	}

	if err := os.Chmod(targetPath, 0777); err != nil {
		klog.V(4).Error(err, fmt.Sprintf("NodePublishVolume cannot change mode for volume %s at target %s on node %s, cleanup", volumeId, targetPath, ns.nodeID))
		mounter.Unmount(targetPath)
		os.RemoveAll(targetPath)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to chown on node %s volume %s path %s info: %v", ns.nodeID, volumeId, targetPath, err))
	}

	klog.V(4).Infof("NodePublishVolume create npvi for volume %s on node %s", volumeId, ns.nodeID)
	err = ns.vh.CreateNodePublishVolumeInfo(volumeId, ns.nodeID, targetPath, rawMount, readOnly)
	if err != nil { // TODO: how to be impodent
		klog.V(4).Error(err, fmt.Sprintf("NodePublishVolume cannot create npvi for volume %s on node %s, cleanup", volumeId, ns.nodeID))
		mounter.Unmount(targetPath)
		os.RemoveAll(targetPath)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to update node %s volume %s path %s info: %v", ns.nodeID, volumeId, targetPath, err))
	}
	klog.V(4).Infof("NodePublishVolume mount volume %s on node %s for path %s succeeded", volumeId, ns.nodeID, targetPath)
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
	volumeId := req.GetVolumeId()

	klog.V(4).Infof("NodeUnpublishVolume try to unpublish volume %s on node %s for path %s", volumeId, ns.nodeID, targetPath)

	vol, err := ns.vh.GetVolume(volumeId)
	if err != nil {
		klog.V(4).Error(err, fmt.Sprintf("NodeUnpublishVolume cannot find volume %s on node %s for path %s", volumeId, ns.nodeID, targetPath))
		return nil, status.Error(codes.NotFound, err.Error())
	}

	if notMnt, err := mount.IsNotMountPoint(mount.New(""), targetPath); err != nil {
		if !os.IsNotExist(err) {
			klog.V(4).Error(err, fmt.Sprintf("NodeUnpublishVolume cannot check mount status volume %s on node %s for path %s", volumeId, ns.nodeID, targetPath))
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if !notMnt {
		err = mount.New("").Unmount(targetPath)
		if err != nil {
			klog.V(4).Error(err, fmt.Sprintf("NodeUnpublishVolume cannot  unmount volume %s on node %s for path %s", volumeId, ns.nodeID, targetPath))
			return nil, status.Error(codes.Internal, err.Error())
		}

		if vol.IsBlock {
			klog.V(4).Infof("NodeUnpublishVolume try detach volume %s device %s", volumeId, vol.VolPath)
			volumeHelpers := volumehelpers.NewBlockVolumePathHandler()
			err := volumeHelpers.DetachFileDevice(vol.VolPath)
			if err != nil {
				klog.V(4).Error(err, "NodeUnpublishVolume detach failed volume %s device %s", volumeId, vol.VolPath)
				return nil, status.Error(codes.Internal, err.Error())
			}
			klog.V(4).Infof("NodeUnpublishVolume detach volume %s device %s succeeded", volumeId, vol.VolPath)
		}
	}

	if err = os.RemoveAll(targetPath); err != nil {
		klog.V(4).Error(err, fmt.Sprintf("NodeUnpublishVolume cannot  remove target path %s on node %s for volume %s", targetPath, ns.nodeID, volumeId))
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = ns.vh.DeleteNodePublishVolumeInfo(volumeId, ns.nodeID, targetPath)
	if err != nil {
		klog.Errorf("NodeUnpublishVolume cannot delete node %s publish volume %s info at path %s: %v", ns.nodeID, volumeId, targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	klog.V(4).Infof("NodeUnpublishVolume unpublish volume %s on node %s for path %s succeeded", volumeId, ns.nodeID, targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	volumeId := req.GetVolumeId()
	if len(volumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeExpandVolume volume ID not provided")
	}

	volumePath := req.GetVolumePath()
	if len(volumePath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeExpandVolume volume path not provided")
	}

	vol, err := ns.vh.GetVolume(volumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	if !vol.IsBlock {
		return &csi.NodeExpandVolumeResponse{}, nil
	}

	klog.V(4).Infof("NodeExpandVolume try to expand volume %s on path %s on node %s", volumeId, volumePath, ns.nodeID)

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
		klog.V(4).Error(err, fmt.Sprintf("NodeExpandVolume cannot find loop back device for volume %s on path %s on node %s", volumeId, volumePath, ns.nodeID))
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get the loop device: %v", err))
	}

	err = volumePathHandler.ReReadFileSize(vol.VolPath)

	if err != nil {
		klog.V(4).Error(err, fmt.Sprintf("NodeExpandVolume cannot reread file size for volume %s on path %s on node %s", volumeId, volumePath, ns.nodeID))
		return nil, status.Error(codes.Internal, fmt.Sprintf("cannot resize backend: %v", err.Error()))
	}

	if resize_fs {
		mounter := mount.New("")
		notMnt, err := mount.IsNotMountPoint(mounter, volumePath)
		if err != nil {
			klog.V(4).Error(err, fmt.Sprintf("NodeExpandVolume cannot check mount status for volume %s on path %s on node %s", volumeId, volumePath, ns.nodeID))
			return nil, status.Errorf(codes.Internal, "NodeExpandVolume failed to check if volume path %q is mounted: %s", volumePath, err)
		}

		if notMnt {
			klog.V(4).Error(errors.New("not mounted"), fmt.Sprintf("NodeExpandVolume volume %s on path %s on node %s is not mounted", volumeId, volumePath, ns.nodeID))
			return nil, status.Errorf(codes.NotFound, "NodeExpandVolume volume path %q is not mounted", volumePath)
		}

		r := volumehelpers.NewResizeFs(&mount.SafeFormatAndMount{Interface: mounter, Exec: utilexec.New()})
		klog.V(4).Infof("NodeExpandVolume Try to expand volume %s at %s", volumeId, volumePath)
		if _, err := r.Resize(loopDevice, volumePath); err != nil {
			return nil, status.Errorf(codes.Internal, "NodeExpandVolume could not resize volume %q (%q):  %v", volumeId, req.GetVolumePath(), err)
		} else {
			klog.V(4).Infof("NodeExpandVolume Volume %s at %s expanded", volumeId, volumePath)
		}
	}
	klog.V(4).Infof("NodeExpandVolume expand volume %s on path %s on node %s succeeded", volumeId, volumePath, ns.nodeID)
	return &csi.NodeExpandVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	volumeId := req.GetVolumeId()
	if len(volumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume ID not provided")
	}

	volumePath := req.GetVolumePath()
	if len(volumePath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats volume path not provided")
	}

	klog.V(4).Infof("NodeGetVolumeStats try to get stats for volume %s on path %s at node %s", volumeId, volumePath, ns.nodeID)

	_, err := ns.vh.GetVolume(volumeId)
	if err != nil {
		klog.V(4).Error(err, fmt.Sprintf("NodeGetVolumeStats get stats for volume %s on path %s at node %s failed", volumeId, volumePath, ns.nodeID))
		return nil, status.Error(codes.NotFound, err.Error())
	}

	_, err = ns.vh.GetNodePublishVolumeInfo(volumeId, ns.nodeID, volumePath)

	if err != nil {
		klog.V(4).Error(err, fmt.Sprintf("NodeGetVolumeStats get stats for volume %s on path %s at node %s failed", volumeId, volumePath, ns.nodeID))
		return nil, status.Errorf(codes.NotFound, "NodeGetVolumeStats volumePath %s is not same as volume's published mount", volumePath)
	}

	fi, err := os.Stat(volumePath)
	if err != nil {
		klog.V(4).Error(err, fmt.Sprintf("NodeGetVolumeStats get stats for volume %s on path %s at node %s failed", volumeId, volumePath, ns.nodeID))
		return nil, status.Errorf(codes.Internal, "NodeGetVolumeStats cannot stat volumepath: %s : %v", volumePath, err)
	}

	var usage []*csi.VolumeUsage
	var condition = &csi.VolumeCondition{}
	if (fi.Mode() & os.ModeDir) == os.ModeDir {
		stats, err := getStatistics(volumePath)
		if err != nil {
			klog.V(4).Error(err, fmt.Sprintf("NodeGetVolumeStats get stats for volume %s on path %s at node %s failed", volumeId, volumePath, ns.nodeID))
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
			klog.V(4).Error(err, fmt.Sprintf("NodeGetVolumeStats get stats for volume %s on path %s at node %s failed", volumeId, volumePath, ns.nodeID))
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
	klog.V(4).Infof("NodeGetVolumeStats get stats for volume %s on path %s at node %s succeeded", volumeId, volumePath, ns.nodeID)
	return &csi.NodeGetVolumeStatsResponse{
		Usage:           usage,
		VolumeCondition: condition,
	}, nil
}

/* Unimplemented methods beyond */

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
