package main

import (
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/kazimsarikaya/csi-sharedhostpath/internal/sharedhostpath"
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
	// Set by the build process
	version = ""
  buildTime = ""
)


func main() {
	flag.Parse()

	if *showVersion {
		baseName := path.Base(os.Args[0])
		fmt.Println(baseName, version)
		return
	}
  handle()
	os.Exit(0)
}

func handle() {
  driver, err := sharedhostpath.NewSharedHostPathDriver(*driverName, *nodeID, *endpoint, *maxVolumesPerNode, version)
	if err != nil {
		fmt.Printf("Failed to initialize driver: %s", err.Error())
		os.Exit(1)
	}
	driver.RunController()
}
