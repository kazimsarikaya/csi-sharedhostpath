package sharedhostpath

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	"os"
	"time"
)

var _ = Describe("Utils Methods Tests", func() {

	Context("Driver Test", func() {

		var vh *VolumeHelper

		BeforeEach(func() {
			var err error
			vh, err = NewVolumeHelper(*dataRoot, *dsn)
			Expect(vh, err).ToNot(BeNil(), "cannot create volume helper")
		})

		AfterEach(func() {
			err := vh.Close()
			Expect(err).To(BeNil(), "cannot close volume helper")
		})

		var _ = Describe("Test open/close", func() {
			It("Should be succeed", func() {})
		})

		var _ = Describe("Test create filesystem volume", func() {
			It("volume should be created", func() {
				vol, err := vh.CreateVolume("d86b0dbb-198f-4642-a4f1-de348da19c99", "test-name-1", "test-pv-1", "test-pvc-1", "test-ns-1", 1<<20, false)
				Expect(vol, err).ToNot(BeNil(), "cannot create volume")
			})
			It("volume folder should be exists", func() {
				Expect(*dataRoot + "/vols/d8/6b/0d/d86b0dbb-198f-4642-a4f1-de348da19c99").Should(BeADirectory())
			})
			It("volume symlink should be exits", func() {
				Expect(*dataRoot + "/syms/test-ns-1/test-pvc-1").Should(BeAnExistingFile())
			})
		})

		var _ = Describe("Test create block volume", func() {
			It("volume should be created", func() {
				vol, err := vh.CreateVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186", "test-name-2", "test-pv-2", "test-pvc-2", "test-ns-2", 1<<20, true)
				Expect(vol, err).ToNot(BeNil(), "cannot create volume")
			})
			It("volume file should be exists", func() {
				Expect(*dataRoot + "/vols/54/9f/7c/549f7cb1-7da1-4b46-97c0-03cbd5a2186").Should(BeAnExistingFile())
			})
			It("volume symlink should be exits", func() {
				Expect(*dataRoot + "/syms/test-ns-2/test-pvc-2").Should(BeAnExistingFile())
			})
		})

		var _ = Describe("Test getting volume", func() {
			var vol *Volume
			var err error
			It("volume should be returned", func() {
				vol, err = vh.GetVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186")
				Expect(vol, err).ToNot(BeNil(), "cannot get volume")
			})
			It("volume id should be equals to queried", func() {
				Expect(vol.VolID).To(Equal("549f7cb1-7da1-4b46-97c0-03cbd5a2186"))
			})
		})

		var _ = Describe("Test deleting volume", func() {
			var vol *Volume
			var err error
			It("volume should be deleted", func() {
				err = vh.DeleteVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186")
				Expect(err).To(BeNil(), "cannot get volume")
			})
			It("volume should not be returned", func() {
				vol, err = vh.GetVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186")
				Expect(vol).To(BeNil(), "cannot delete volume")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
			})
			It("volume file should not be exists", func() {
				Expect(*dataRoot + "/vols/54/9f/7c/549f7cb1-7da1-4b46-97c0-03cbd5a2186").ShouldNot(BeAnExistingFile())
			})
			It("volume symlink should not be exits", func() {
				Expect(*dataRoot + "/syms/test-ns-2/test-pvc-2").ShouldNot(BeAnExistingFile())
			})
		})

		var _ = Describe("Get volume id by name", func() {
			var volid string
			var err error
			It("volume id should be found", func() {
				volid, err = vh.GetVolumeIdByName("test-name-1")
				Expect(volid).ShouldNot(Equal(""))
				Expect(err).To(BeNil(), "error occured")
			})
			It("volume id shoul be expected", func() {
				Expect(volid).To(Equal("d86b0dbb-198f-4642-a4f1-de348da19c99"), "volid is not expected")
			})
		})

		var _ = Describe("Rebuild symlins", func() {
			It("rebuild should be work", func() {
				err := vh.ReBuildSymLinks()
				Expect(err).To(BeNil(), "error occured")
			})
			It("symlink should be exists", func() {
				Expect(*dataRoot + "/syms/test-ns-1/test-pvc-1").Should(BeAnExistingFile())
			})
		})

		var _ = Describe("Cleanup dangling volumes", func() {
			os.MkdirAll(*dataRoot+"/vols/54/9f/7c/549f7cb1-7da1-4b46-97c0-03cbd5a2186", 0750)
			It("rebuild should be work", func() {
				err := vh.CleanUpDanglingVolumes()
				Expect(err).To(BeNil(), "error occured")
			})
			It("volume should not be exists", func() {
				Expect(*dataRoot + "/vols/54/9f/7c/549f7cb1-7da1-4b46-97c0-03cbd5a2186").ShouldNot(BeADirectory())
			})
		})

		var _ = Describe("Update node info last seen", func() {
			It("create last seen", func() {
				err := vh.UpdateNodeInfoLastSeen("testnode", time.Now())
				Expect(err).To(BeNil(), "error occured when creating node last seen")
			})
			It("update last seen after 1 second", func() {
				time.Sleep(time.Second)
				err := vh.UpdateNodeInfoLastSeen("testnode", time.Now())
				Expect(err).To(BeNil(), "error occured when creating node last seen")
			})
		})

		var _ = Describe("Get node info", func() {
			It("create last seen", func() {
				err := vh.UpdateNodeInfoLastSeen("testnode", time.Now())
				Expect(err).To(BeNil(), "error occured when creating node last seen")
			})
			It("get node info", func() {
				ni, err := vh.GetNodeInfo("testnode", 1000)
				Expect(err).To(BeNil(), "error occured when getting node info")
				Expect(ni).NotTo(BeNil(), "node info is nil")
			})
			It("get node info", func() {
				time.Sleep(time.Second)
				ni, err := vh.GetNodeInfo("testnode", 0)
				Expect(err).To(BeNil(), "error occured when getting node info")
				Expect(ni).To(BeNil(), "node info is not nil")
			})
		})

		var _ = Describe("Test NodePublishVolumeInfo operations", func() {
			var cont bool = true
			It("create node publish volume info", func() {
				err := vh.CreateNodePublishVolumeInfo("test-volume", "test-node", false)
				Expect(err).To(BeNil(), "cannot create npvi")
				if err != nil {
					cont = false
				}
			})
			It("get node publish volume info", func() {
				if !cont {
					Skip("cannot test without create")
				}
				nvpi, err := vh.GetNodePublishVolumeInfo("test-volume", "test-node")
				Expect(err).To(BeNil(), "cannot get npvi")
				Expect(nvpi).NotTo(BeNil(), "nvpi is nil")
			})
			It("get node publish volume info with non exists volume", func() {
				if !cont {
					Skip("cannot test without succeed create")
				}
				nvpi, err := vh.GetNodePublishVolumeInfo("test-volume-noexists", "test-node")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
				Expect(nvpi).To(BeNil(), "nvpi is not  nil")
			})
			It("get node publish volume info with non exists node", func() {
				if !cont {
					Skip("cannot test without create")
				}
				nvpi, err := vh.GetNodePublishVolumeInfo("test-volume", "test-node-noexists")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
				Expect(nvpi).To(BeNil(), "nvpi is nil")
			})
			It("delete node publish volume info", func() {
				if !cont {
					Skip("cannot test without create")
				}
				err := vh.DeleteNodePublishVolumeInfo("test-volume", "test-node")
				Expect(err).To(BeNil(), "cannot delete npvi")
				if err != nil {
					cont = false
				}
			})
			It("get node publish volume info with deleted", func() {
				if !cont {
					Skip("cannot test without succeed delete")
				}
				nvpi, err := vh.GetNodePublishVolumeInfo("test-volume", "test-node")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
				Expect(nvpi).To(BeNil(), "nvpi is nil")
			})
		})
	})
})
