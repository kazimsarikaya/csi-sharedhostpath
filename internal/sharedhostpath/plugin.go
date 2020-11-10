package sharedhostpath

import (
	"errors"
	"fmt"
	klog "k8s.io/klog/v2"
	"os"
)

type sharedHostPath struct {
	name              string
	nodeID            string
	version           string
	endpoint          string
	maxVolumesPerNode int64
	vh                *VolumeHelper

	ids *identityServer
	ns  *nodeServer
	cs  *controllerServer

	server *nonBlockingGRPCServer
}

var (
	vendorVersion = "dev"
)

func NewSharedHostPathDriver(driverName, nodeID, endpoint, dataRoot, dsn string, maxVolumesPerNode int64, version string) (*sharedHostPath, error) {
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
		return nil, fmt.Errorf("failed to create DataRoot: %v", err)
	}

	vh, err := NewVolumeHelper(dataRoot, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection: %v", err)
	}

	klog.V(5).Infof("Driver: %v ", driverName)
	klog.V(5).Infof("Version: %s", vendorVersion)

	return &sharedHostPath{
		name:              driverName,
		version:           vendorVersion,
		nodeID:            nodeID,
		endpoint:          endpoint,
		maxVolumesPerNode: maxVolumesPerNode,
		vh:                vh,
	}, nil
}

func (shp *sharedHostPath) RunController() {
	// Create GRPC servers
	shp.ids = NewIdentityServer(shp.name, true, shp.version)
	shp.cs = NewControllerServer(shp.nodeID, shp.vh)

	shp.server = NewNonBlockingGRPCServer()
	shp.server.Start(shp.endpoint, shp.ids, shp.cs, nil)
	shp.server.Wait()
}

func (shp *sharedHostPath) RunNode() {
	// Create GRPC servers
	shp.ids = NewIdentityServer(shp.name, false, shp.version)
	shp.ns = NewNodeServer(shp.nodeID, shp.maxVolumesPerNode, shp.vh)

	shp.server = NewNonBlockingGRPCServer()
	shp.server.Start(shp.endpoint, shp.ids, nil, shp.ns)
	shp.server.Wait()
}

func (shp *sharedHostPath) RunBoth() {
	shp.ids = NewIdentityServer(shp.name, true, shp.version)
	shp.cs = NewControllerServer(shp.nodeID, shp.vh)
	shp.ns = NewNodeServer(shp.nodeID, shp.maxVolumesPerNode, shp.vh)

	shp.server = NewNonBlockingGRPCServer()
	shp.server.Start(shp.endpoint, shp.ids, shp.cs, shp.ns)
	shp.server.Wait()
}

func (shp *sharedHostPath) Stop() {
	shp.server.Stop()
}

func (shp *sharedHostPath) ForceStop() {
	shp.server.ForceStop()
}
