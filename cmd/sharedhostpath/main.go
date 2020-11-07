package main

import (
	"flag"
	"fmt"
	"github.com/kazimsarikaya/csi-sharedhostpath/internal/sharedhostpath"
	"os"
	"path"
)

func init() {
	flag.Set("logtostderr", "true")
}

var (
	endpoint          = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	driverName        = flag.String("drivername", "sharedhostpath.csi.k8s.io", "name of the driver")
	nodeID            = flag.String("nodeid", "", "node id")
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
		vh, err := sharedhostpath.NewVolumeHelper(sharedhostpath.DataRoot)
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
		driver, err := sharedhostpath.NewSharedHostPathDriver(*driverName, *nodeID, *endpoint, *maxVolumesPerNode, version)
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
