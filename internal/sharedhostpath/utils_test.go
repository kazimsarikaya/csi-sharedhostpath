package sharedhostpath

import (
	"flag"
	"github.com/stretchr/testify/assert"
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

func TestCreateStorageFS(t *testing.T) {
	vh, err := NewVolumeHelper(*dataRoot, *dsn)
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	_, err = vh.CreateVolume("d86b0dbb-198f-4642-a4f1-de348da19c99", "test-name-1", "test-pv-1", "test-pvc-1", "test-ns-1", 1<<20, false)
	if err != nil {
		t.Errorf("create volume failed %s", err)
	}
	assert.DirExistsf(t, *dataRoot+"/vols/d8/6b/0d/d86b0dbb-198f-4642-a4f1-de348da19c99", "no volume: failed")
}

func TestCreateStorageBlock(t *testing.T) {
	vh, err := NewVolumeHelper(*dataRoot, *dsn)
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	_, err = vh.CreateVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186", "test-name-2", "test-pv-2", "test-pvc-2", "test-ns-2", 1<<20, true)
	if err != nil {
		t.Errorf("create volume failed %s", err)
	}
	assert.FileExistsf(t, *dataRoot+"/vols/54/9f/7c/549f7cb1-7da1-4b46-97c0-03cbd5a2186", "no volume: failed")
}

func TestGetVolume(t *testing.T) {
	vh, err := NewVolumeHelper(*dataRoot, *dsn)
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	vol, err := vh.GetVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186")
	if err != nil {
		t.Errorf("get volume failed %s", err)
	}
	if vol != nil {
		assert.EqualValuesf(t, "549f7cb1-7da1-4b46-97c0-03cbd5a2186", vol.VolID, "returned volume is different")
	}
}

func TestDeleteVolume(t *testing.T) {
	vh, err := NewVolumeHelper(*dataRoot, *dsn)
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	err = vh.DeleteVolume("549f7cb1-7da1-4b46-97c0-03cbd5a2186")
	if err != nil {
		t.Errorf("delete volume failed %s", err)
	}
	assert.NoFileExistsf(t, *dataRoot+"/vols/54/9f/7c/549f7cb1-7da1-4b46-97c0-03cbd5a2186", "volume delete if failed")
	assert.NoFileExistsf(t, *dataRoot+"/syms/test-ns-2/test-pvc-2", "symlink delete if failed")
}

func TestGetVolumeIdByName(t *testing.T) {
	vh, err := NewVolumeHelper(*dataRoot, *dsn)
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	volid, err := vh.GetVolumeIdByName("test-name-1")
	if err != nil {
		t.Errorf("get volume id by name failed %s", err)
	}
	assert.EqualValuesf(t, "d86b0dbb-198f-4642-a4f1-de348da19c99", volid, "returned volume is different")
}

func TestReBuildSymLinks(t *testing.T) {
	vh, err := NewVolumeHelper(*dataRoot, *dsn)
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	err = vh.ReBuildSymLinks()
	if err != nil {
		t.Errorf("cannot rebuild symlinks: %v", err)
	}
}

func TestCleanUpDanglingVolumes(t *testing.T) {
	vh, err := NewVolumeHelper(*dataRoot, *dsn)
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	err = vh.CleanUpDanglingVolumes()
	if err != nil {
		t.Errorf("clean up dangling volumes failed %s", err)
	}
	assert.NoFileExistsf(t, *dataRoot+"/vols/d8/6b/0d/d86b0dbb-198f-4642-a4f1-de348da19c99", "clean up dangling volumes failed")
}
