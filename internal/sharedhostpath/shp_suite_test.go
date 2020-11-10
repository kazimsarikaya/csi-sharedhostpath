package sharedhostpath

import (
	"flag"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	klog "k8s.io/klog/v2"
	"os"
	"testing"
)

var (
	dataRoot = flag.String("dataroot", "", "node id")
	dsn      = flag.String("dsn", "", "postgres data dsn")
)

func init() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	klog.SetOutput(os.Stdout)
}

func TestDriver(t *testing.T) {
	// Setup the full driver and its environment
	if *dataRoot == "" {
		t.Fatalf("dataroot parameter is empty")
	}
	if *dsn == "" {
		t.Fatalf("dns parameter is empty")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared Host Path Driver Suite")
}
