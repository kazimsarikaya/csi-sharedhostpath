package sharedhostpath

import (
	"fmt"
	"github.com/golang/glog"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"runtime"
	"time"

	utilexec "k8s.io/utils/exec"
)

const (
	dbname       = "definitions.db"
	volume_base  = "vols"
	symlink_base = "syms"
)

type VolumeHelper struct {
	db_path   string
	vols_path string
	syms_path string
	db        *gorm.DB
}

type Volume struct {
	StorageID uint64 `gorm:"autoIncrement"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	VolID     string         `gorm:"uniqueIndex; not null"`
	VolName   string         `gorm:"uniqueIndex; not null"`
	PVName    string         `gorm:"not null"`
	PVCName   string         `gorm:"not null"`
	NSName    string         `gorm:"index; not null"`
	Capacity  uint64
	IsBlock   bool
}

func NewVolumeHelper(basepath string) (*VolumeHelper, error) {
	basepath, _ = filepath.Abs(basepath)
	vols_path := filepath.Join(basepath, volume_base)
	err := os.MkdirAll(vols_path, 0755)
	if err != nil {
		glog.Errorf("cannot create vols path: %s %v", vols_path, err)
		return nil, err
	}

	syms_path := filepath.Join(basepath, symlink_base)
	err = os.MkdirAll(syms_path, 0755)
	if err != nil {
		glog.Errorf("cannot create vols path: %s %v", syms_path, err)
		return nil, err
	}

	dbPath := filepath.Join(basepath, dbname)

	var need_create bool

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		need_create = true
	}
	dsn := fmt.Sprintf("%s?cache=shared&_journal_mode=wal", dbPath)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		glog.Errorf("can not create db file %s %v", dbPath, err)
		return nil, err
	}

	vh := &VolumeHelper{
		db_path:   dbPath,
		vols_path: vols_path,
		syms_path: syms_path,
		db:        db,
	}

	if need_create {
		_, err := vh.CreateDB()
		if err != nil {
			glog.Errorf("can not create db %v", err)
			return nil, err
		}
	}

	return vh, nil
}

func (vh *VolumeHelper) DeleteDB() error {
	sqlfs := fmt.Sprintf("%s*", vh.db_path)
	files, err := filepath.Glob(sqlfs)
	if err != nil {
		glog.Errorf("cannot fetch db files: %s %v", vh.db_path, err)
		return err
	}
	for _, f := range files {
		err = os.Remove(f)
		if err == nil || os.IsNotExist(err) {
			glog.Infof("the db file removed: %s", f)
		} else {
			glog.Errorf("the db could not be removed: %s %v", f, err)
		}
	}
	return err
}

func (vh *VolumeHelper) CreateDB() (bool, error) {

	err := vh.db.AutoMigrate(&Volume{})

	if err != nil {
		glog.Errorf("cannot create db schema on db %s %v", vh.db_path, err)
		return false, err
	}
	return true, nil
}

func (vh *VolumeHelper) CreateVolume(volid, volname, pvname, pvcname, nsname string,
	capacity uint64, isblock bool) (bool, error) {
	var err error = nil

	tx := vh.db.Begin()

	defer func() {
		if err != nil {
			glog.Errorf("there is an error: %v, tran will be rollbacked", err)
			tx.Rollback()
		}
	}()

	storage := Volume{VolID: volid, VolName: volname, PVName: pvname,
		PVCName: pvcname, NSName: nsname, Capacity: capacity, IsBlock: isblock}

	result := tx.Create(&storage)

	if result.Error != nil {
		glog.Errorf("cannot insert volume data into db: %v", result.Error)
		return false, result.Error
	}

	volume_path := filepath.Join(vh.vols_path, volid)

	if isblock {
		executor := utilexec.New()
		cap_str := fmt.Sprintf("%d", capacity)

		if runtime.GOOS == "linux" {
			_, err = executor.Command("fallocate", "-l", cap_str, volume_path).CombinedOutput()
		} else if runtime.GOOS == "darwin" {
			_, err = executor.Command("mkfile", "-n", cap_str, volume_path).CombinedOutput()
		}

		if err != nil {
			glog.Errorf("cannot create volume file: %s %v", volume_path, err)
			return false, err
		}
	} else {
		err := os.MkdirAll(volume_path, 0755)
		if err != nil {
			glog.Errorf("cannot create volume dir: %s %v", volume_path, err)
			return false, err
		}
	}

	symlink_dir := filepath.Join(vh.syms_path, nsname)
	symlink_file := filepath.Join(symlink_dir, pvcname)
	err = os.MkdirAll(symlink_dir, 0750)
	if err == nil {
		if err == nil {
			os.Symlink(volume_path, symlink_file)
		}
	}

	tx.Commit()
	err = vh.db.Error
	if err != nil {
		glog.Errorf("cannot create volume dir: %s %v", volume_path, err)
		os.RemoveAll(volume_path)
		os.RemoveAll(symlink_file)
		return false, err
	}
	return true, nil
}

func (vh *VolumeHelper) GetVolume(volid string) (Volume, error) {
	var vol Volume
	vh.db.Where("vol_id = ?", volid).First(&vol)
	return vol, vh.db.Error
}

func (vh *VolumeHelper) DeleteVolume(volid string) error {
	vol, err := vh.GetVolume(volid)
	if err != nil {
		glog.Errorf("cannot get volume %s: %v", volid, err)
		return err
	}

	tx := vh.db.Begin()
	defer func() {
		if err != nil {
			glog.Errorf("there is an error while deleting volume: %v, tran will be rollbacked", err)
			tx.Rollback()
		}
	}()

	vh.db.Where("vol_id = ?", vol.VolID).Delete(&Volume{})

	volume_path := filepath.Join(vh.vols_path, volid)
	symlink_dir := filepath.Join(vh.syms_path, vol.NSName)
	symlink_file := filepath.Join(symlink_dir, vol.PVCName)

	os.Remove(symlink_file)
	err = os.RemoveAll(volume_path)

	if err != nil {
		tx.Commit()
		err = vh.db.Error
	}
	return err
}

func (vh *VolumeHelper) GetVolumeIdByName(volname string) (string, error) {
	var vol Volume
	vh.db.Where("vol_name = ?", volname).First(&vol)
	return vol.VolID, vh.db.Error
}

func (vh *VolumeHelper) ReBuildSymLinks() error {
	var vols []Volume

	err := os.RemoveAll(vh.syms_path)
	if err != nil {
		glog.Errorf("cannot remove syms folder: %v", err)
		return err
	}
	err = os.MkdirAll(vh.syms_path, 0750)
	if err != nil {
		glog.Errorf("cannot recreate syms folder: %v", err)
		return err
	}

	result := vh.db.Find(&vols)

	if result.Error != nil {
		glog.Errorf("cannot get volumes from db: %v", err)
		return err
	}

	for _, vol := range vols {
		volume_path := filepath.Join(vh.vols_path, vol.VolID)
		symlink_dir := filepath.Join(vh.syms_path, vol.NSName)
		symlink_file := filepath.Join(symlink_dir, vol.PVCName)

		err = os.MkdirAll(symlink_dir, 0750)
		if err == nil {
			err = os.Symlink(volume_path, symlink_file)
		}
	}

	return err
}

/*
func (vh *VolumeHelper) CleanUpDanglingVolumes() error {

}
*/
