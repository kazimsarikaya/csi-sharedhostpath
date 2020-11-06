package sharedhostpath

import (
	"testing"
)

func TestDeleteDB(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	_, err = vh.DeleteDB()
	if err != nil {
		t.Errorf("create db failed %s", err)
	}
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
	_, err = vh.CreateVolume("test", "test", "test", "test", "test", 1<<20, false)
	if err != nil {
		t.Errorf("create storage failed %s", err)
	}
}

func TestCreateStorageBlock(t *testing.T) {
	vh, err := NewVolumeHelper("./tmp/")
	if err != nil {
		t.Errorf("cannot create volume helper: %v", err)
	}
	_, err = vh.CreateVolume("test2", "test2", "test2", "test2", "test2", 1<<20, true)
	if err != nil {
		t.Errorf("create storage failed %s", err)
	}
}
