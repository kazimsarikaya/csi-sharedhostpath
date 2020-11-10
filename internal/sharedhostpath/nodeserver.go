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
)

type nodeServer struct {
	nodeID            string
	maxVolumesPerNode int64
	caps              []*csi.NodeServiceCapability
	vh                *VolumeHelper
}

func NewNodeServer(nodeId string, maxVolumesPerNode int64, vh *VolumeHelper) *nodeServer {
	return &nodeServer{
		nodeID:            nodeId,
		maxVolumesPerNode: maxVolumesPerNode,
		caps:              []*csi.NodeServiceCapability{},
		vh:                vh,
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
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}
	targetPath := req.GetTargetPath()
	volumeID := req.GetVolumeId()

	vol, err := ns.vh.GetVolume(volumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	mounter := mount.New("")

	if req.GetVolumeCapability().GetBlock() != nil {
		if !vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "cannot publish a non-block volume as block volume")
		}

		volPathHandler := volumepathhandler.VolumePathHandler{}

		// Get loop device from the volume path.
		loopDevice, err := volPathHandler.GetLoopDevice(vol.VolPath)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get the loop device: %v", err))
		}

		// Check if the target path exists. Create if not present.
		_, err = os.Lstat(targetPath)
		if os.IsNotExist(err) {
			var f *os.File
			f, err = os.OpenFile(targetPath, os.O_CREATE, 0640)
			if err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create target path: %s: %v", targetPath, err))
			}
			if err := f.Close(); err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create target path: %s: %v", targetPath, err))
			}
		}
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to check if the target block file exists: %v", err)
		}

		// Check if the target path is already mounted. Prevent remounting.
		notMount, err := mount.IsNotMountPoint(mounter, targetPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, status.Errorf(codes.Internal, "error checking path %s for mount: %s", targetPath, err)
			}
			notMount = true
		}
		if !notMount {
			// It's already mounted.
			klog.V(5).Infof("Skipping bind-mounting subpath %s: already mounted", targetPath)
			return &csi.NodePublishVolumeResponse{}, nil
		}

		options := []string{"bind"}
		if err := mounter.Mount(loopDevice, targetPath, "", options); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to mount block device: %s at %s: %v", loopDevice, targetPath, err))
		}
	} else if req.GetVolumeCapability().GetMount() != nil {
		volume_context := req.GetVolumeContext()
		var vtype string
		var found bool
		if vtype, found = volume_context[typeParameter]; !found {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("required parameter not found: %s", typeParameter))
		}
		if vtype == "disk" && !vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "cannot publish a non-block volume as disk volume")
		}
		if vtype == "folder" && vol.IsBlock {
			return nil, status.Error(codes.InvalidArgument, "cannot publish a block volume as folder volume")
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

		if !notMnt {
			return &csi.NodePublishVolumeResponse{}, nil
		}

		readOnly := req.GetReadonly()
		options := []string{}
		if readOnly {
			options = append(options, "ro")
		}

		if vtype == "folder" {
			options = append(options, "bind")
			if err := mounter.Mount(vol.VolPath, targetPath, "", options); err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("failed to mount device: %s at %s: %s", vol.VolPath, targetPath, err.Error()))
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
			err = formatAndMount.FormatAndMount(loopDevice, targetPath, fsType, options)
			if err != nil {
				return nil, status.Error(codes.Internal, fmt.Sprintf("failed to mount device: %s at %s: %s", vol.VolPath, targetPath, err.Error()))
			}
		} else {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid volume type: %s", vtype))
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

	vol, err := ns.vh.GetVolume(volumeID)
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

	if vol.IsBlock {
		volPathHandler := volumepathhandler.NewBlockVolumePathHandler()
		err := volPathHandler.DetachFileDevice(vol.VolPath)
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

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, in *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
