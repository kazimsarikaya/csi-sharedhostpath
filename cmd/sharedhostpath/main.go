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

package main

import (
	"flag"
	"fmt"
	"github.com/kazimsarikaya/csi-sharedhostpath/internal/sharedhostpath"
	klog "k8s.io/klog/v2"
	"os"
	"path"
)

func init() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
}

var (
	endpoint          = flag.String("endpoint", "unix:///tmp/csi.sock", "CSI endpoint")
	driverName        = flag.String("drivername", "sharedhostpath.csi.k8s.io", "name of the driver")
	nodeID            = flag.String("nodeid", "", "node id")
	dataRoot          = flag.String("dataroot", "/csi-data-dir", "node id")
	dsn               = flag.String("dsn", "", "postgres data dsn")
	maxVolumesPerNode = flag.Int64("maxvolumespernode", 0, "limit of volumes per node")
	showVersion       = flag.Bool("version", false, "Show version.")
	controller        = flag.Bool("controller", false, "Run as controller.")
	node              = flag.Bool("node", false, "Run as node.")
	rebuildsymlinks   = flag.Bool("job-rebuildsymlinks", false, "Rebuild sym links.")
	cleanupdangling   = flag.Bool("job-cleanupdangling", false, "Cleanup dangling volumes.")
	// Set by the build process
	version   = ""
	buildTime = ""
)

func main() {
	flag.Parse()

	if *showVersion {
		baseName := path.Base(os.Args[0])
		fmt.Println(baseName, version, buildTime)
		return
	}
	handle()
	os.Exit(0)
}

func handle() {
	var f_cnt int = 0
	if *controller {
		f_cnt++
	}
	if *node {
		f_cnt++
	}
	if *rebuildsymlinks {
		f_cnt++
	}
	if *cleanupdangling {
		f_cnt++
	}
	if f_cnt != 1 {
		fmt.Printf("only one of controller,node,job-rebuildsymlinks,job-cleanupdangling flags should be set.\n")
		os.Exit(1)
	}

	if *rebuildsymlinks || *cleanupdangling {
		vh, err := sharedhostpath.NewVolumeHelper(*dataRoot, *dsn)
		if err != nil {
			fmt.Printf("cannot create volume helper: %v", err)
			os.Exit(1)
		}
		if *rebuildsymlinks {
			vh.ReBuildSymLinks()
		} else {
			vh.CleanUpDanglingVolumes()
		}

	} else {
		driver, err := sharedhostpath.NewSharedHostPathDriver(*driverName, *nodeID, *endpoint, *dataRoot, *dsn, *maxVolumesPerNode, version)
		if err != nil {
			fmt.Printf("Failed to initialize driver: %s\n", err.Error())
			os.Exit(1)
		}

		if *controller {
			driver.RunController()
		} else if *node {
			driver.RunNode()
		}
	}

}
