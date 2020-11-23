// +build linux

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
	"github.com/kubernetes-csi/csi-test/v4/pkg/sanity"
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

		var folder_config = sanity.NewTestConfig()
		folder_config.Address = address
		folder_config.TestVolumeSize = 1 * 1024 * 1024 * 1024
		folder_config.TestVolumeParameters = map[string]string{
			"sharedhostpath.sanaldiyar.com/type": "folder",
			"csi.storage.k8s.io/pvc/name":        "sanity-pvc",
			"csi.storage.k8s.io/pvc/namespace":   "sanity-ns",
			"csi.storage.k8s.io/pv/name":         "sanity-pv",
		}

		var disk_config = sanity.NewTestConfig()
		disk_config.Address = address
		disk_config.TestVolumeSize = 1 * 1024 * 1024 * 1024
		disk_config.TestVolumeParameters = map[string]string{
			"sharedhostpath.sanaldiyar.com/type":   "disk",
			"sharedhostpath.sanaldiyar.com/fsType": "xfs",
			"csi.storage.k8s.io/pvc/name":          "sanity-pvc",
			"csi.storage.k8s.io/pvc/namespace":     "sanity-ns",
			"csi.storage.k8s.io/pv/name":           "sanity-pv",
		}

		BeforeEach(func() {
			os.RemoveAll("/csi-data-dir")
		})

		AfterEach(func() {
			os.RemoveAll("/csi-data-dir")
		})

		Describe("CSI sanity Folder", func() {
			sanity.GinkgoTest(&folder_config)
		})

		Describe("CSI sanity Disk", func() {
			sanity.GinkgoTest(&disk_config)
		})
	})

})
