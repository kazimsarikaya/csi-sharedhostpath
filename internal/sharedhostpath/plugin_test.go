// +build linux

package sharedhostpath

import (
	"github.com/kubernetes-csi/csi-test/pkg/sanity"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
)

var shp *sharedHostPath
var address string = "unix:///tmp/csi.socket"

var _ = BeforeSuite(func() {
	var err error
	shp, err = NewSharedHostPathDriver("sharedhostpath.sanaldiyar.com", "testnode", address, *dataRoot, *dsn, 0, "dev")
	Expect(shp, err).ToNot(BeNil(), "cannot create driver")
	go func() {
		shp.RunBoth()
	}()
})

var _ = AfterSuite(func() {
	shp.Stop()
})

var _ = Describe("SharedHostPathDriver", func() {
	Context("Driver Test", func() {

		var folder_config = &sanity.Config{
			TargetPath:     os.TempDir() + "/csi-mount",
			StagingPath:    os.TempDir() + "/csi-staging",
			Address:        address,
			SecretsFile:    "",
			TestVolumeSize: 1 * 1024 * 1024 * 1024,
			IDGen:          &sanity.DefaultIDGenerator{},
			TestVolumeParameters: map[string]string{
				"sharedhostpath.sanaldiyar.com/type": "folder",
				"csi.storage.k8s.io/pvc/name":        "sanity-pvc",
				"csi.storage.k8s.io/pvc/namespace":   "sanity-ns",
				"csi.storage.k8s.io/pv/name":         "sanity-pv",
			},
		}

		var disk_config = &sanity.Config{
			TargetPath:     os.TempDir() + "/csi-mount",
			StagingPath:    os.TempDir() + "/csi-staging",
			Address:        address,
			SecretsFile:    "",
			TestVolumeSize: 1 * 1024 * 1024 * 1024,
			IDGen:          &sanity.DefaultIDGenerator{},
			TestVolumeParameters: map[string]string{
				"sharedhostpath.sanaldiyar.com/type":   "disk",
				"sharedhostpath.sanaldiyar.com/fsType": "xfs",
				"csi.storage.k8s.io/pvc/name":          "sanity-pvc",
				"csi.storage.k8s.io/pvc/namespace":     "sanity-ns",
				"csi.storage.k8s.io/pv/name":           "sanity-pv",
			},
		}

		BeforeEach(func() {
			os.RemoveAll("/csi-data-dir")
		})

		AfterEach(func() {
			os.RemoveAll("/csi-data-dir")
		})

		Describe("CSI sanity Folder", func() {
			sanity.GinkgoTest(folder_config)
		})

		Describe("CSI sanity Disk", func() {
			sanity.GinkgoTest(disk_config)
		})
	})

})