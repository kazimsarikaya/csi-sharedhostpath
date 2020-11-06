package sharedhostpath

import (
	"errors"
	"fmt"
	"os"

	"github.com/golang/glog"
)

type sharedHostPath struct {
	name              string
	nodeID            string
	version           string
	endpoint          string
	maxVolumesPerNode int64

	ids *identityServer
	ns  *nodeServer
	cs  *controllerServer
}

var (
	vendorVersion = "dev"
)

const (
	dataRoot = "/csi-data-dir"
)

func NewSharedHostPathDriver(driverName, nodeID, endpoint string, maxVolumesPerNode int64, version string) (*sharedHostPath, error) {
	if driverName == "" {
		return nil, errors.New("no driver name provided")
	}

	if nodeID == "" {
		return nil, errors.New("no node id provided")
	}

	if endpoint == "" {
		return nil, errors.New("no driver endpoint provided")
	}
	if version != "" {
		vendorVersion = version
	}

	if err := os.MkdirAll(dataRoot, 0750); err != nil {
		return nil, fmt.Errorf("failed to create dataRoot: %v", err)
	}

	glog.Infof("Driver: %v ", driverName)
	glog.Infof("Version: %s", vendorVersion)

	return &sharedHostPath{
		name:              driverName,
		version:           vendorVersion,
		nodeID:            nodeID,
		endpoint:          endpoint,
		maxVolumesPerNode: maxVolumesPerNode,
	}, nil
}

func (shp *sharedHostPath) RunController() {
	// Create GRPC servers
	shp.ids = NewIdentityServer(shp.name, true, shp.version)
	shp.cs = NewControllerServer(shp.nodeID)

	s := NewNonBlockingGRPCServer()
	s.Start(shp.endpoint, shp.ids, shp.cs, nil)
	s.Wait()
}

func (shp *sharedHostPath) RunNode() {
	// Create GRPC servers
	shp.ids = NewIdentityServer(shp.name, false, shp.version)
	shp.ns = NewNodeServer(shp.nodeID, shp.maxVolumesPerNode)

	s := NewNonBlockingGRPCServer()
	s.Start(shp.endpoint, shp.ids, nil, shp.ns)
	s.Wait()
}
