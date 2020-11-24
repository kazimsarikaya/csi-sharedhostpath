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
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	utilexec "k8s.io/utils/exec"
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

		Describe("Test open/close", func() {
			It("Should be succeed", func() {})
		})

		Describe("Test create filesystem volume", func() {
			It("volume should be created", func() {
				vol, err := vh.CreateVolume("d86b0dbb-198f-4642-a4f1-de348da19c99", "test-name-1", "test-pv-1", "test-pvc-1", "test-ns-1", 1<<30, false)
				Expect(vol, err).ToNot(BeNil(), "cannot create volume")
				Expect(*dataRoot+"/vols/d8/6b/0d/d86b0dbb-198f-4642-a4f1-de348da19c99").Should(BeADirectory(), "volume folder should be exists")
				Expect(*dataRoot+"/syms/test-ns-1/test-pvc-1").Should(BeAnExistingFile(), "volume symlink should be exits")
			})
		})

		Describe("Test create block volume", func() {
			It("volume should be created", func() {
				vol, err := vh.CreateVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186", "test-name-2", "test-pv-2", "test-pvc-2", "test-ns-2", 1<<30, true)
				Expect(vol, err).ToNot(BeNil(), "cannot create volume")
				Expect(*dataRoot+"/vols/54/9f/7c/549f7cb1-7da1-4b46-97c0-03cbd5a2186").Should(BeAnExistingFile(), "volume file should be exists")
				Expect(*dataRoot+"/syms/test-ns-2/test-pvc-2").Should(BeAnExistingFile(), "volume symlink should be exits")
			})

			It("Test Getting volume", func() {
				vol, err := vh.GetVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186")
				Expect(vol, err).ToNot(BeNil(), "cannot get volume")
				Expect(vol.VolID).To(Equal("549f7cb1-7da1-4b46-97c0-03cbd5a2186"), "volume id should be equals to queried")
			})

			It("Test Deleting Volume", func() {
				By("volume should be deleted")
				err := vh.DeleteVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186")
				Expect(err).To(BeNil(), "cannot get volume")

				By("volume should not be returned")
				vol, err := vh.GetVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186")
				Expect(vol).To(BeNil(), "cannot delete volume")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))

				By("Any data should not be exists")
				Expect(*dataRoot+"/vols/54/9f/7c/549f7cb1-7da1-4b46-97c0-03cbd5a2186").ShouldNot(BeAnExistingFile(), "volume file should not be exists")
				Expect(*dataRoot+"/syms/test-ns-2/test-pvc-2").ShouldNot(BeAnExistingFile(), "volume symlink should not be exits")
			})
		})

		Describe("Get volume id by name", func() {
			It("volume id should be found", func() {
				volid, err := vh.GetVolumeIdByName("test-name-1")
				Expect(volid).ShouldNot(Equal(""))
				Expect(err).To(BeNil(), "error occured")
				Expect(volid).To(Equal("d86b0dbb-198f-4642-a4f1-de348da19c99"), "volid is not expected")
			})
		})

		Describe("Rebuild symlins", func() {
			It("rebuild should be work", func() {
				err := vh.ReBuildSymLinks()
				Expect(err).To(BeNil(), "error occured")
				Expect(*dataRoot + "/syms/test-ns-1/test-pvc-1").Should(BeAnExistingFile())
			})
		})

		Describe("Cleanup dangling volumes", func() {
			It("rebuild should be work", func() {
				By("create dummy folder")
				os.MkdirAll(*dataRoot+"/vols/54/9f/7c/549f7cb1-7da1-4b46-97c0-03cbd5a2186", 0750)

				By("cleanup dangling volumes")
				err := vh.CleanUpDanglingVolumes()
				Expect(err).To(BeNil(), "error occured")
				Expect(*dataRoot + "/vols/54/9f/7c/549f7cb1-7da1-4b46-97c0-03cbd5a2186").ShouldNot(BeADirectory())

				By("cleanup")
				vh.DeleteVolume("d86b0dbb-198f-4642-a4f1-de348da19c99")
			})
		})

		Describe("Create/Update/Get node info", func() {
			It("should work", func() {
				By("create bode info")
				err := vh.UpdateNodeInfoLastSeen("testnode", time.Now())
				Expect(err).To(BeNil(), "error occured when creating node last seen")

				By("wait 0.5 second")
				time.Sleep(time.Millisecond * 500)
				err = vh.UpdateNodeInfoLastSeen("testnode", time.Now())
				Expect(err).To(BeNil(), "error occured when updating node last seen")

				By("get node info")
				ni, err := vh.GetNodeInfo("testnode", 2000)
				Expect(err).To(BeNil(), "error occured when getting node info")
				Expect(ni).NotTo(BeNil(), "node info is nil")

				By("wait 1 second")
				time.Sleep(time.Second)

				By("get node info")
				ni, err = vh.GetNodeInfo("testnode", 0)
				Expect(err).To(BeNil(), "error occured when getting node info")
				Expect(ni).To(BeNil(), "node info is not nil")
			})
		})

		Describe("Test ControllerPublishVolumeInfo operations", func() {
			It("should work", func() {
				By("create dummy cpvi")
				err := vh.CreateControllerPublishVolumeInfo("test-volume", "test-node", false)
				Expect(err).To(BeNil(), "cannot create cpvi")

				By("get controller publish volume info")
				cpvi, err := vh.GetControllerPublishVolumeInfo("test-volume", "test-node")
				Expect(err).To(BeNil(), "cannot get cpvi")
				Expect(cpvi).NotTo(BeNil(), "cpvi is nil")

				By("get controller publish volume info with non exists volume")

				cpvi, err = vh.GetControllerPublishVolumeInfo("test-volume-noexists", "test-node")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
				Expect(cpvi).To(BeNil(), "cpvi is not  nil")

				By("get controller publish volume info with non exists node")
				cpvi, err = vh.GetControllerPublishVolumeInfo("test-volume", "test-node-noexists")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
				Expect(cpvi).To(BeNil(), "cpvi is nil")

				By("delete controller publish volume info")
				err = vh.DeleteControllerPublishVolumeInfo("test-volume", "test-node")
				Expect(err).To(BeNil(), "cannot delete cpvi")

				By("get controller publish volume info with deleted")
				cpvi, err = vh.GetControllerPublishVolumeInfo("test-volume", "test-node")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
				Expect(cpvi).To(BeNil(), "cpvi is nil")
			})
		})

		Describe("Test NodePublishVolumeInfo operations", func() {
			It("should work", func() {
				By("create dummy npvi")
				err := vh.CreateNodePublishVolumeInfo("test-volume", "test-node", "/dummy/mount/point", false, false)
				Expect(err).To(BeNil(), "cannot create npvi")

				By("get node publish volume info")
				npvi, err := vh.GetNodePublishVolumeInfo("test-volume", "test-node", "/dummy/mount/point")
				Expect(err).To(BeNil(), "cannot get npvi")
				Expect(npvi).NotTo(BeNil(), "npvi is nil")

				By("get node publish volume info with non exists volume")

				npvi, err = vh.GetNodePublishVolumeInfo("test-volume-noexists", "test-node", "/dummy/mount/point")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
				Expect(npvi).To(BeNil(), "npvi is not  nil")

				By("get node publish volume info with non exists node")
				npvi, err = vh.GetNodePublishVolumeInfo("test-volume", "test-node-noexists", "/dummy/mount/point")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
				Expect(npvi).To(BeNil(), "npvi is nil")

				By("get node publish volume info with non exists mount point")
				npvi, err = vh.GetNodePublishVolumeInfo("test-volume", "test-node", "/dummy/no-mount/point")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
				Expect(npvi).To(BeNil(), "npvi is nil")

				By("delete node publish volume info")
				err = vh.DeleteNodePublishVolumeInfo("test-volume", "test-node", "/dummy/mount/point")
				Expect(err).To(BeNil(), "cannot delete npvi")

				By("get node publish volume info with deleted")
				npvi, err = vh.GetNodePublishVolumeInfo("test-volume", "test-node", "/dummy/mount/point")
				Expect(err).Should(MatchError(gorm.ErrRecordNotFound))
				Expect(npvi).To(BeNil(), "npvi is nil")
			})
		})

		Describe("Expand Volume", func() {
			It("test folder volume creation should succeed", func() {
				volname := "43eb5928-b7fe-48ef-895f-d259a92a9072"
				var vol *Volume
				var err error

				By("create dummy volume")
				vol, err = vh.CreateVolume(volname, "test-name-3", "test-pv-3", "test-pvc-3", "test-ns-3", 1<<30, false)
				Expect(vol, err).ToNot(BeNil(), "cannot create folder volume")

				By("expand volume")
				err = vh.UpdateVolumeCapacity(vol, 2<<30)
				Expect(err).To(BeNil(), "cannot delete cpvi")

				By("new volume size should be 2gib")
				vol, err = vh.GetVolume(volname)
				Expect(vol, err).ToNot(BeNil(), "cannot get volume")
				Expect(vol.Capacity).To(Equal(int64(2<<30)), "vol size did not expended")

				By("clean up volume")
				vh.DeleteVolume(volname)
			})

			It("Expand disk type volume succeed", func() {
				volname := "2f92e632-1a6a-42f0-957f-be63a97e9261"
				var vol *Volume
				var err error

				By("create dummy volume")
				vol, err = vh.CreateVolume(volname, "test-name-4", "test-pv-4", "test-pvc-4", "test-ns-4", 1<<30, true)
				Expect(vol, err).ToNot(BeNil(), "cannot create folder volume")

				By("expand volume")
				err = vh.UpdateVolumeCapacity(vol, 2<<30)
				Expect(err).To(BeNil(), "cannot delete cpvi")

				By("new volume size should be 2gib")
				vol, err = vh.GetVolume(volname)
				Expect(vol, err).ToNot(BeNil(), "cannot get volume")
				Expect(vol.Capacity).To(Equal(int64(2<<30)), "vol size did not expended")

				By("file size should be 2gib")
				fi, _ := os.Stat(vol.VolPath)
				Expect(fi.Size()).To(Equal(int64(2<<30)), "file size did not expended")

				By("clean up volume")
				vh.DeleteVolume(volname)
			})
		})

		Describe("get volume details", func() {
			It("get volume details should fail with non exists volume", func() {
				vd, err := vh.GetVolumeWithDetail("any-volume")
				Expect(err).To(BeNil(), "error at getting volume detail")
				Expect(vd).To(BeNil(), "volume should not be exists")
			})

			It("get volume details for folder type should work", func() {
				volname := "26a136a1-7dcf-4dd7-b306-83d64afdc7e9"
				By("create dummy volume")
				vol, err := vh.CreateVolume(volname, "test-name-5", "test-pv-5", "test-pvc-5", "test-ns-5", 1<<30, false)
				Expect(vol, err).ToNot(BeNil(), "cannot create folder volume")

				By("get volume detail")
				vd, err := vh.GetVolumeWithDetail(volname)
				Expect(err).To(BeNil(), "error at getting volume detail")
				Expect(vd).NotTo(BeNil(), "volume detail should be exists")
				Expect(vd["volumeId"]).To(Equal(volname))
				Expect(vd["condition_abnormal"]).NotTo(BeTrue(), "condition should be ok")

				By("if volume folder deleted, condition should be false")
				os.RemoveAll(vol.VolPath)
				vd, err = vh.GetVolumeWithDetail(volname)
				Expect(err).To(BeNil(), "error at getting volume detail")
				Expect(vd).NotTo(BeNil(), "volume detail should be exists")
				Expect(vd["volumeId"]).To(Equal(volname))
				Expect(vd["condition_abnormal"]).To(BeTrue(), "condition should not be ok")

				By("cleanup volume")
				vh.DeleteVolume(volname)
			})

			It("get volume details for disk type should work", func() {
				volname := "f715058b-3ae9-4f59-877d-3800354d51d5"
				By("create dummy volume")
				vol, err := vh.CreateVolume(volname, "test-name-6", "test-pv-6", "test-pvc-6", "test-ns-6", 1<<30, true)
				Expect(vol, err).ToNot(BeNil(), "cannot create folder volume")

				By("get volume detail")
				vd, err := vh.GetVolumeWithDetail(volname)
				Expect(err).To(BeNil(), "error at getting volume detail")
				Expect(vd).NotTo(BeNil(), "volume detail should be exists")
				Expect(vd["volumeId"]).To(Equal(volname))
				Expect(vd["condition_abnormal"]).NotTo(BeTrue(), "condition should be ok")

				By("if volume file resized, condition should be false")
				executor := utilexec.New()
				cap_str := fmt.Sprintf("seek=%d", 1<<20)
				vp_str := fmt.Sprintf("of=%s", vol.VolPath)
				_, err = executor.Command("dd", "if=/dev/null", "bs=1", "count=0", cap_str, vp_str).CombinedOutput()
				vd, err = vh.GetVolumeWithDetail(volname)
				Expect(err).To(BeNil(), "error at getting volume detail")
				Expect(vd).NotTo(BeNil(), "volume detail should be exists")
				Expect(vd["volumeId"]).To(Equal(volname))
				Expect(vd["condition_abnormal"]).To(BeTrue(), "condition should not be ok")

				By("if volume file deleted, condition should be false")
				os.RemoveAll(vol.VolPath)
				vd, err = vh.GetVolumeWithDetail(volname)
				Expect(err).To(BeNil(), "error at getting volume detail")
				Expect(vd).NotTo(BeNil(), "volume detail should be exists")
				Expect(vd["volumeId"]).To(Equal(volname))
				Expect(vd["condition_abnormal"]).To(BeTrue(), "condition should not be ok")

				By("cleanup volume")
				vh.DeleteVolume(volname)
			})
		})
	})
})
