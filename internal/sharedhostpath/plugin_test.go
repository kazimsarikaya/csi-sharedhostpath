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
	"crypto/sha256"
	"github.com/kazimsarikaya/csi-sharedhostpath/internal/volumehelpers"
	"github.com/kubernetes-csi/csi-test/v4/pkg/sanity"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io"
	klog "k8s.io/klog/v2"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
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
		folder_config.TestVolumeExpandSize = 2 * folder_config.TestVolumeSize
		folder_config.TestVolumeParameters = map[string]string{
			"sharedhostpath.sanaldiyar.com/type": "folder",
			"csi.storage.k8s.io/pvc/name":        "sanity-pvc",
			"csi.storage.k8s.io/pvc/namespace":   "sanity-ns",
			"csi.storage.k8s.io/pv/name":         "sanity-pv",
		}

		var disk_config = sanity.NewTestConfig()
		disk_config.Address = address
		disk_config.TestVolumeSize = 1 * 1024 * 1024 * 1024
		disk_config.TestVolumeExpandSize = 2 * disk_config.TestVolumeSize
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

	Context("Test Disk resize", func() {
		executor := utilexec.New()
		mounter := mount.New("")
		formatAndMount := mount.SafeFormatAndMount{Interface: mounter, Exec: executor}
		volumePathHandler := volumehelpers.VolumePathHandler{}

		AfterEach(func() {
			mounter.Unmount("/tmp/resize-xfs")
			volumePathHandler.DetachFileDevice("/tmp/testdisk.raw")
			os.RemoveAll("/tmp/testdisk.raw")
			os.RemoveAll("/tmp/resize-xfs")

		})

		Describe("XFS resize test", func() {

			It("should work", func() {
				By("Create 1gib disk")
				output, err := executor.Command("dd", "if=/dev/null", "bs=1", "count=0", "seek=1G", "of=/tmp/testdisk.raw").CombinedOutput()
				Expect(err).To(BeNil(), "cannot create disk file")
				klog.Infof("create disk output: %v", string(output))

				By("create mounting dir")
				err = os.MkdirAll("/tmp/resize-xfs", 0750)
				Expect(err).To(BeNil(), "cannot mount folder")

				By("attach file to device")
				loopDevice, err := volumePathHandler.AttachFileDevice("/tmp/testdisk.raw")
				Expect(err).To(BeNil(), "cannot attach file")
				klog.Infof("file attached to %v", loopDevice)

				By("Format and mount")
				err = formatAndMount.FormatAndMount(loopDevice, "/tmp/resize-xfs", "xfs", []string{"nouuid"})
				Expect(err).To(BeNil(), "cannot format and mount")
				notMnt, err := mount.IsNotMountPoint(mounter, "/tmp/resize-xfs")
				Expect(err).To(BeNil(), "cannot check mount point")
				Expect(notMnt).NotTo(BeTrue(), "mount failed")

				By("create file and get hash")
				output, err = executor.Command("dd", "if=/dev/urandom", "bs=1M", "count=100", "of=/tmp/resize-xfs/test.bin").CombinedOutput()
				Expect(err).To(BeNil(), "cannot create random file")
				klog.Infof("create disk output: %s", string(output))
				rf, err := os.Open("/tmp/resize-xfs/test.bin")
				Expect(err).To(BeNil(), "cannot open random file")
				hasher := sha256.New()
				_, err = io.Copy(hasher, rf)
				rf.Close()
				old_hash := hasher.Sum(nil)

				By("check sizes")
				old_bsize, err := getBlockDeviceSize(loopDevice)
				Expect(err).To(BeNil(), "cannot get block device size")
				old_stats, err := getStatistics("/tmp/resize-xfs")
				Expect(err).To(BeNil(), "cannot get mounted volume statistics")

				By("resize")
				output, err = executor.Command("dd", "if=/dev/null", "bs=1", "count=0", "seek=2G", "of=/tmp/testdisk.raw").CombinedOutput()
				Expect(err).To(BeNil(), "cannot expand disk file")
				klog.Infof("create disk output: %s", string(output))
				err = volumePathHandler.ReReadFileSize("/tmp/testdisk.raw")
				Expect(err).To(BeNil(), "cannot reread disk file size")
				r := volumehelpers.NewResizeFs(&mount.SafeFormatAndMount{Interface: mounter, Exec: executor})
				_, err = r.Resize(loopDevice, "/tmp/resize-xfs")
				Expect(err).To(BeNil(), "cannot resize xfs")

				By("check new sizes first time")
				new_bsize1, err := getBlockDeviceSize(loopDevice)
				Expect(err).To(BeNil(), "cannot get block device size")
				new_stats1, err := getStatistics("/tmp/resize-xfs")
				Expect(err).To(BeNil(), "cannot get mounted volume statistics")

				By("reattach device and mount")
				err = mounter.Unmount("/tmp/resize-xfs")
				Expect(err).To(BeNil(), "cannot unmount")
				err = volumePathHandler.DetachFileDevice("/tmp/testdisk.raw")
				Expect(err).To(BeNil(), "cannot detach device")
				loopDevice, err = volumePathHandler.AttachFileDevice("/tmp/testdisk.raw")
				Expect(err).To(BeNil(), "cannot attach file")
				klog.Infof("file attached to %v", loopDevice)
				err = formatAndMount.FormatAndMount(loopDevice, "/tmp/resize-xfs", "xfs", []string{"nouuid"})
				Expect(err).To(BeNil(), "cannot format and mount")
				notMnt, err = mount.IsNotMountPoint(mounter, "/tmp/resize-xfs")
				Expect(err).To(BeNil(), "cannot check mount point")
				Expect(notMnt).NotTo(BeTrue(), "mount failed")

				By("check new sizes second time")
				new_bsize2, err := getBlockDeviceSize(loopDevice)
				Expect(err).To(BeNil(), "cannot get block device size")
				new_stats2, err := getStatistics("/tmp/resize-xfs")
				Expect(err).To(BeNil(), "cannot get mounted volume statistics")

				By("recalculate hash")
				rf, err = os.Open("/tmp/resize-xfs/test.bin")
				Expect(err).To(BeNil(), "cannot open random file")
				hasher = sha256.New()
				_, err = io.Copy(hasher, rf)
				rf.Close()
				new_hash := hasher.Sum(nil)

				By("Crosschecks")
				Expect(old_bsize).To(Equal(int64(1<<30)), "old block size is not 1g")
				Expect(new_bsize1).To(Equal(int64(2<<30)), "first new block size is not 2g")
				Expect(new_bsize2).To(Equal(int64(2<<30)), "second new block size is not 2g")
				Expect(old_stats.totalBytes*2).To(BeNumerically("<=", new_stats1.totalBytes), "volume size didnot expanded")
				Expect(new_stats2.totalBytes).To(Equal(new_stats1.totalBytes), "volume size dismatch")
				Expect(old_hash).To(Equal(new_hash), "file content dismatch")
			})
		})
	})
})
