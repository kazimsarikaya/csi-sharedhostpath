package sharedhostpath

import (
	"crypto/md5"
	"errors"
	"fmt"
	"github.com/golang/glog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"io"
	utilexec "k8s.io/utils/exec"
	"os"
	"path/filepath"
	"time"
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
	VolPath   string `gorm:"uniqueIndex; not null"`
}

func NewVolumeHelper(basepath string) (*VolumeHelper, error) {
	basepath, _ = filepath.Abs(basepath)
	vols_path := filepath.Join(basepath, volume_base)
	err := os.MkdirAll(vols_path, 0750)
	if err != nil {
		glog.Errorf("cannot create vols path: %s %v", vols_path, err)
		return nil, err
	}

	syms_path := filepath.Join(basepath, symlink_base)
	err = os.MkdirAll(syms_path, 0750)
	if err != nil {
		glog.Errorf("cannot create vols path: %s %v", syms_path, err)
		return nil, err
	}

	dbPath := filepath.Join(basepath, dbname)

	var need_create bool

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		need_create = true
	}
	dsn := fmt.Sprintf("%s?cache=shared&_journal_mode=wal&_busy_timeout=1000", dbPath)
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
	glog.V(5).Infof("volume helper is created")
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
			glog.V(5).Infof("the db file removed: %s", f)
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
	glog.V(5).Info("database schema created")
	return true, nil
}

func (vh *VolumeHelper) CreateVolume(volid, volname, pvname, pvcname, nsname string,
	capacity uint64, isblock bool) (*Volume, error) {
	var err error = nil

	h := md5.New()
	io.WriteString(h, volid)
	hash := h.Sum(nil)
	prefix := fmt.Sprintf("%s/%02x/%02x/%02x", vh.vols_path, hash[0], hash[1], hash[2])
	prefix = filepath.FromSlash(prefix)

	err = os.MkdirAll(prefix, 0750)
	if err != nil {
		glog.Errorf("cannot create vols prefix: %s %v", prefix, err)
		return nil, err
	}

	volume_path := filepath.Join(prefix, volid)

	tx := vh.db.Begin()
	if tx == nil || vh.db.Error != nil {
		glog.Errorf("cannot start transaction: %v", err)
		return nil, vh.db.Error
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			glog.Errorf("an error accured while trans, rollback performed %v", err)
		}
	}()

	vol := Volume{VolID: volid, VolName: volname, PVName: pvname,
		PVCName: pvcname, NSName: nsname,
		Capacity: capacity, IsBlock: isblock,
		VolPath: volume_path}

	result := tx.Create(&vol)

	if result.Error != nil {
		tx.Rollback()
		glog.Errorf("cannot insert volume data into db: %v", result.Error)
		return nil, result.Error
	}

	_, err = vol.PopulateVolumeIfRequired()
	if err != nil {
		tx.Rollback()
		return nil, errors.New(fmt.Sprintf("cannot populate volume: %v", err.Error()))
	}

	symlink_dir := filepath.Join(vh.syms_path, vol.NSName)
	symlink_file := filepath.Join(symlink_dir, vol.PVCName)
	err = os.MkdirAll(symlink_dir, 0750)
	if err == nil {
		if err == nil {
			os.Symlink(vol.VolPath, symlink_file)
		}
	}

	err = tx.Commit().Error
	if err != nil {
		tx.Rollback()
		glog.Errorf("cannot create volume dir: %s %v", volume_path, err)
		os.RemoveAll(volume_path)
		os.RemoveAll(symlink_file)
		return nil, err
	} else {
		glog.V(5).Infof("volume %s created for %s/%s", vol.VolID, vol.NSName, vol.PVCName)
	}
	return &vol, nil
}

func (vh *VolumeHelper) GetVolume(volid string) (*Volume, error) {
	var vol Volume
	result := vh.db.Where("vol_id = ?", volid).First(&vol)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	return &vol, result.Error
}

func (vh *VolumeHelper) DeleteVolume(volid string) error {
	vol, err := vh.GetVolume(volid)
	if err != nil {
		glog.Errorf("cannot get volume %s: %v", volid, err)
		return err
	}

	volume_path := vol.VolPath

	tx := vh.db.Begin()
	if tx == nil || vh.db.Error != nil {
		glog.Errorf("cannot start transaction: %v", err)
		return vh.db.Error
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			glog.Errorf("an error accured while trans, rollback performed %v", err)
		}
	}()

	vh.db.Where("vol_id = ?", vol.VolID).Delete(&Volume{})

	symlink_dir := filepath.Join(vh.syms_path, vol.NSName)
	symlink_file := filepath.Join(symlink_dir, vol.PVCName)

	os.Remove(symlink_file)
	err = os.RemoveAll(volume_path)

	if err != nil {
		glog.Errorf("there is an error while deleting volume: %v, tran will be rollbacked", err)
		tx.Rollback()
		glog.Errorf("volume %s cannot be deleted for %s/%s", vol.VolID, vol.NSName, vol.PVCName)
	} else {
		err = tx.Commit().Error
	}
	if err == nil {
		glog.V(5).Infof("volume %s deleted for %s/%s", vol.VolID, vol.NSName, vol.PVCName)
	}

	return err
}

func (vh *VolumeHelper) GetVolumeIdByName(volname string) (string, error) {
	var vol Volume
	result := vh.db.Where("vol_name = ?", volname).First(&vol)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return "", result.Error
	}
	return vol.VolID, result.Error
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
		volume_path := vol.VolPath
		symlink_dir := filepath.Join(vh.syms_path, vol.NSName)
		symlink_file := filepath.Join(symlink_dir, vol.PVCName)

		err = os.MkdirAll(symlink_dir, 0750)
		if err == nil {
			err = os.Symlink(volume_path, symlink_file)
		}
	}
	if err == nil {
		glog.V(5).Infof("all symlinks rebuilded")
	}
	return err
}

func (vh *VolumeHelper) CleanUpDanglingVolumes() error {
	var vols []Volume
	vh.db.Unscoped().Where("deleted_at is not null").Find(&vols)
	err := vh.db.Error
	if err != nil {
		glog.Errorf("cannot get deleted volumes from db: %v", err)
		return err
	}

	// Phase1 delete vols from disk if deleted from db
	for _, vol := range vols {
		volume_path := filepath.Join(vh.vols_path, vol.VolID)
		if _, err := os.Stat(volume_path); err == nil {
			err = os.RemoveAll(volume_path)
			if err != nil {
				glog.Errorf("cannot deleted volumes from disk: %v", err)
			}
		}
	}

	// Phase2 delete vols from disk if not exists on db
	pattern := fmt.Sprintf("%s/*/*/*/*", vh.vols_path)
	fs, err := filepath.Glob(pattern)
	if err != nil {
		glog.Errorf("cannot read volumes (volid) from disk: %v", err)
		return err
	}
	for _, f := range fs {
		var vols []Volume
		result := vh.db.Where("vol_path = ?", f).Find(&vols)
		if result.RowsAffected == 0 {
			err = os.RemoveAll(f)
			if err != nil {
				glog.Errorf("cannot deleted volumes from disk: %v", err)
			}
		}
	}

	if err == nil {
		glog.V(5).Infof("all dangling volumes are deleted")
	}
	// Phase3 rebuild links
	return vh.ReBuildSymLinks()
}

func (vol *Volume) PopulateVolumeIfRequired() (bool, error) {
	var err error
	_, err = os.Lstat(vol.VolPath)
	if os.IsNotExist(err) {
		if vol.IsBlock {
			executor := utilexec.New()
			cap_str := fmt.Sprintf("seek=%d", vol.Capacity)
			vp_str := fmt.Sprintf("of=%s", vol.VolPath)
			var output []byte
			output, err = executor.Command("dd", "if=/dev/null", "bs=1", "count=0", cap_str, vp_str).CombinedOutput()
			if err != nil {
				return false, errors.New(fmt.Sprintf("cannot create volume file: %s %v %s", vol.VolPath, err.Error(), string(output)))
			}
		} else {
			err := os.MkdirAll(vol.VolPath, 0750)
			if err != nil {
				return false, errors.New(fmt.Sprintf("cannot create volume dir: %s %v", vol.VolPath, err.Error()))
			}
		}
		return true, nil
	} else {
		if err == nil {
			return false, nil
		}
		return false, errors.New(fmt.Sprintf("cannot stat volume data: %v", err.Error()))
	}
	return false, nil
}
