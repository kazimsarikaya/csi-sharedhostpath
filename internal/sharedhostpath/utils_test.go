package sharedhostpath

import (
	"flag"
	"github.com/stretchr/testify/assert"
	"testing"
)

func init() {
	flag.Set("logtostderr", "true")
}

func TestCreateDB(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	_, err = vh.CreateDB()
	if err != nil {
		t.Errorf("create db failed %s", err)
	}
}

func TestCreateStorageFS(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	_, err = vh.CreateVolume("test-id-1", "test-name-1", "test-pv-1", "test-pvc-1", "test-ns-1", 1<<20, false)
	if err != nil {
		t.Errorf("create volume failed %s", err)
	}
	assert.DirExistsf(t, "./tmp/vols/23/db/3d/test-id-1", "no volume: failed")
}

func TestCreateStorageBlock(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	_, err = vh.CreateVolume("test2", "test2", "test2", "test2", "test2", 1<<20, true)
	if err != nil {
		t.Errorf("create volume failed %s", err)
	}
	assert.FileExistsf(t, "./tmp/vols/ad/02/34/test2", "no volume: failed")
}

func TestGetVolume(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	vol, err := vh.GetVolume("test2")
	if err != nil {
		t.Errorf("get volume failed %s", err)
	}
	if vol != nil {
		assert.EqualValuesf(t, "test2", vol.VolID, "returned volume is different")
	}
}

func TestDeleteVolume(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	err = vh.DeleteVolume("test2")
	if err != nil {
		t.Errorf("delete volume failed %s", err)
	}
	assert.NoFileExistsf(t, "./tmp/vols/ad/02/34/test2", "volume delete if failed")
	assert.NoFileExistsf(t, "./tmp/syms/test2/test2", "symlink delete if failed")
}

func TestGetVolumeIdByName(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	volid, err := vh.GetVolumeIdByName("test-name-1")
	if err != nil {
		t.Errorf("get volume id by name failed %s", err)
	}
	assert.EqualValuesf(t, "test-id-1", volid, "returned volume is different")
}

func TestReBuildSymLinks(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	err = vh.ReBuildSymLinks()
	if err != nil {
		t.Errorf("cannot rebuild symlinks: %v", err)
	}
}

func TestDeleteDB(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	err = vh.DeleteDB()
	if err != nil {
		t.Errorf("delete db failed %s", err)
	}
}

func TestCleanUpDanglingVolumes(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	err = vh.CleanUpDanglingVolumes()
	if err != nil {
		t.Errorf("clean up dangling volumes failed %s", err)
	}
	assert.NoFileExistsf(t, "./tmp/vols/23/db/3d/test-id-1", "clean up dangling volumes failed")
}
