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
	vendorVersion   = "dev"
	fstypeParameter = "/fsType"
	typeParameter   = "/type"
)

func NewSharedHostPathDriver(driverName, nodeID, endpoint, dataRoot, dsn string, maxVolumesPerNode int64, version string) (*sharedHostPath, error) {
	if driverName == "" {
		return nil, errors.New("no driver name provided")
	}

	fstypeParameter = driverName + fstypeParameter
	typeParameter = driverName + typeParameter

	if nodeID == "" {
		return nil, errors.New("no node id provided")
	}

	if endpoint == "" {
		return nil, errors.New("no driver endpoint provided")
	}

	if dataRoot == "" {
		return nil, errors.New("no data root provided")
	}

	if dsn == "" {
		return nil, errors.New("no dsn (connstring) provided")
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
	shp.vh.Close()
}

func (shp *sharedHostPath) ForceStop() {
	shp.server.ForceStop()
	shp.vh.Close()
}
