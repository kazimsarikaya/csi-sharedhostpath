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
	"errors"
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	klog "k8s.io/klog/v2"
	utilexec "k8s.io/utils/exec"
	"os"
	"path/filepath"
	"time"
)

const (
	dbname       = "definitions.db"
	volume_base  = "vols"
	symlink_base = "syms"
	MiB          = 1 << 20
	GiB          = 1 << 30
)

type volumeStatistics struct {
	availableBytes, totalBytes, usedBytes    int64
	availableInodes, totalInodes, usedInodes int64
}

type VolumeHelper struct {
	vols_path string
	syms_path string
	db        *gorm.DB
	dsn       string
}

type Volume struct {
	StorageID int64 `gorm:"autoIncrement"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	VolID     string         `gorm:"uniqueIndex; not null"`
	VolName   string         `gorm:"index; not null"`
	PVName    string         `gorm:"not null"`
	PVCName   string         `gorm:"not null"`
	NSName    string         `gorm:"index; not null"`
	Capacity  int64
	IsBlock   bool
	VolPath   string `gorm:"uniqueIndex; not null"`
}

type NodeInfo struct {
	ID       string    `gorm:"primaryKey"`
	LastSeen time.Time `sql:"DEFAULT:current_timestamp"`
}

type ControllerPublishVolumeInfo struct {
	StorageID int64 `gorm:"autoIncrement"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	VolID     string         `gorm:"index; not null"`
	NodeID    string         `gorm:"index; not null"`
	ReadOnly  bool
}

type NodePublishVolumeInfo struct {
	StorageID int64 `gorm:"autoIncrement"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	VolID     string         `gorm:"index; not null"`
	NodeID    string         `gorm:"index; not null"`
	MountPath string
	RawMount  bool
	ReadOnly  bool
}

func NewVolumeHelper(dataRoot, dsn string) (*VolumeHelper, error) {
	dataRoot, _ = filepath.Abs(dataRoot)
	vols_path := filepath.Join(dataRoot, volume_base)
	err := os.MkdirAll(vols_path, 0750)
	if err != nil {
		klog.V(5).Error(err, "NewVolumeHelper cannot create vols path: %s", vols_path)
		return nil, err
	}

	syms_path := filepath.Join(dataRoot, symlink_base)
	err = os.MkdirAll(syms_path, 0750)
	if err != nil {
		klog.V(5).Error(err, "NewVolumeHelper cannot cannot create vols path: %s", syms_path)
		return nil, err
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		klog.V(5).Error(err, "NewVolumeHelper cannot can not create db file %s", dsn)
		return nil, err
	}
	sqlDB, err := db.DB()
	sqlDB.SetMaxOpenConns(5)
	klog.V(5).Infof("NewVolumeHelper db connection established")

	err = db.AutoMigrate(&Volume{}, &NodeInfo{}, &ControllerPublishVolumeInfo{}, &NodePublishVolumeInfo{})

	if err != nil {
		klog.V(5).Error(err, "NewVolumeHelper cannot create db schema on dsn %s", dsn)
		return nil, err
	}
	klog.V(5).Info("NewVolumeHelper database schema created")

	vh := &VolumeHelper{
		vols_path: vols_path,
		syms_path: syms_path,
		db:        db,
		dsn:       dsn,
	}

	klog.V(5).Infof("NewVolumeHelper volume helper is created")
	return vh, nil
}

func (vh *VolumeHelper) CreateVolume(volid, volname, pvname, pvcname, nsname string, capacity int64, isblock bool) (*Volume, error) {
	var err error = nil

	prefix := fmt.Sprintf("%s/%s/%s/%s", vh.vols_path, volid[0:2], volid[2:4], volid[4:6])
	prefix = filepath.FromSlash(prefix)

	err = os.MkdirAll(prefix, 0750)
	if err != nil {
		klog.V(5).Error(err, "CreateVolume cannot create vols prefix: %s", prefix)
		return nil, err
	}

	volume_path := filepath.Join(prefix, volid)

	tx := vh.db.Begin()
	if tx == nil || vh.db.Error != nil {
		klog.V(5).Error(err, "CreateVolume cannot start transaction")
		return nil, vh.db.Error
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			klog.V(5).Error(errors.New(fmt.Sprintf("%v", err)), "CreateVolume an error accured while trans, rollback performed")
		}
	}()

	vol := Volume{VolID: volid, VolName: volname, PVName: pvname,
		PVCName: pvcname, NSName: nsname,
		Capacity: capacity, IsBlock: isblock,
		VolPath: volume_path}

	result := tx.Create(&vol)

	if result.Error != nil {
		tx.Rollback()
		klog.V(5).Error(err, "CreateVolume cannot insert volume data into db")
		return nil, result.Error
	}

	_, err = vol.PopulateVolumeIfRequired()
	if err != nil {
		tx.Rollback()
		klog.V(5).Error(err, "CreateVolume cannot populate volume")
		return nil, err
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
		klog.V(5).Error(err, "CreateVolume cannot create volume dir: %s %v", volume_path)
		os.RemoveAll(volume_path)
		os.RemoveAll(symlink_file)
		return nil, err
	} else {
		klog.V(5).Infof("CreateVolume volume %s created for %s/%s", vol.VolID, vol.NSName, vol.PVCName)
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

func (vh *VolumeHelper) UpdateVolumeCapacity(vol *Volume, capacity int64) error {

	tx := vh.db.Begin()
	err := vh.db.Error
	if tx == nil || err != nil {
		klog.V(5).Error(err, "UpdateVolumeCapacity cannot start transaction")
		return vh.db.Error
	}

	oldCapacity := vol.Capacity
	vol.Capacity = capacity
	err = vh.db.Model(&Volume{}).Where("vol_id = ?", vol.VolID).Update("capacity", vol.Capacity).Error
	if err != nil {
		tx.Rollback()
		return err
	}

	if vol.IsBlock {
		volume_path := vol.VolPath
		fi, err := os.Stat(volume_path)

		if err != nil {
			tx.Rollback()
			errstr := fmt.Sprintf("UpdateVolumeCapacity rollback: expanding volume error: cannot stat file: %s : %v", volume_path, err)
			err = errors.New(errstr)
			klog.V(5).Error(err, "UpdateVolumeCapacity error occured")
			return err
		}

		if fi.Size() != oldCapacity {
			tx.Rollback()
			errstr := fmt.Sprintf("UpdateVolumeCapacity rollback: expanding volume error: file size dismatch: db-> %v os-> %v", oldCapacity, fi.Size())
			err = errors.New(errstr)
			klog.V(5).Error(err, "UpdateVolumeCapacity error occured")
			return err
		}

		executor := utilexec.New()
		cap_str := fmt.Sprintf("seek=%d", vol.Capacity)
		vp_str := fmt.Sprintf("of=%s", vol.VolPath)
		var output []byte
		output, err = executor.Command("dd", "if=/dev/null", "bs=1", "count=0", cap_str, vp_str).CombinedOutput()
		if err != nil {
			tx.Rollback()
			errstr := fmt.Sprintf("UpdateVolumeCapacity cannot expand volume file: %s %v %s", vol.VolPath, err.Error(), string(output))
			err = errors.New(errstr)
			klog.V(5).Error(err, "UpdateVolumeCapacity error occured")
			return err
		}

	}

	if err != nil {
		klog.V(5).Error(err, "UpdateVolumeCapacity there is an error while expanding volume, tran will be rollbacked")
		tx.Rollback()
		klog.V(5).Error(err, "UpdateVolumeCapacity volume %s cannot be expanded for %s/%s", vol.VolID, vol.NSName, vol.PVCName)
	} else {
		err = tx.Commit().Error
	}
	if err == nil {
		klog.V(5).Infof("UpdateVolumeCapacity volume %s expanded for %s/%s", vol.VolID, vol.NSName, vol.PVCName)
	}

	return err
}

func (vh *VolumeHelper) GetVolumeWithDetail(volid string) (map[string]interface{}, error) {
	var vol Volume
	var err error

	klog.V(5).Infof("GetVolumeWithDetail volume details will be obtained for %s", volid)

	result := vh.db.Where("vol_id = ?", volid).First(&vol)
	err = result.Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		} else {
			klog.V(5).Error(err, "GetVolumeWithDetail cannot get volume list from db")
			return nil, err
		}
	}

	vol_detail := make(map[string]interface{})

	var node_ids []string
	var cpvis []ControllerPublishVolumeInfo
	result = vh.db.Where("vol_id = ?", vol.VolID).Find(&cpvis)
	err = result.Error
	if err != nil {
		klog.V(5).Error(err, "GetVolumeWithDetail cannot get volume published node list from db")
		return nil, err
	} else {
		for _, cpvi := range cpvis {
			node_ids = append(node_ids, cpvi.NodeID)
		}
	}

	vol_detail["published_node_ids"] = node_ids
	params := make(map[string]string)
	params[pvNameKey] = vol.PVName
	params[pvcNameKey] = vol.PVCName
	params[pvcNamespaceKey] = vol.NSName
	params[typeParameter] = "folder"
	if vol.IsBlock {
		params[typeParameter] = "disk"
	}
	vol_detail["parameters"] = params
	vol_detail["capacity"] = vol.Capacity

	vol_detail["condition_abnormal"] = true
	var fi os.FileInfo
	fi, err = os.Stat(vol.VolPath)
	if err != nil {
		vol_detail["condition_msg"] = err.Error()
	} else {
		if vol.IsBlock && fi.Size() != vol.Capacity {
			vol_detail["condition_msg"] = "file size dismatch"
		} else {
			vol_detail["condition_msg"] = "ok"
			vol_detail["condition_abnormal"] = false
		}
	}
	vol_detail["volumeId"] = vol.VolID

	klog.V(5).Infof("GetVolumeWithDetail volume detail obtained for %s", volid)
	return vol_detail, nil
}

func (vh *VolumeHelper) GetVolumeCount() (int, error) {
	var vols []Volume
	vh.db.Select("vol_id").Find(&vols)
	err := vh.db.Error
	if err != nil {
		klog.V(5).Error(err, "GetVolumeCount cannot get volume count from db")
		return 0, err
	}
	vc := len(vols)
	klog.V(5).Infof("GetVolumeCount volume count: %d", vc)
	return vc, nil
}

func (vh *VolumeHelper) GetVolumesWithDetail(offset, limit int) ([]map[string]interface{}, error) {
	var vols []Volume
	var err error
	klog.V(5).Infof("GetVolumesWithDetail volume details will be obtained from %v to %v", offset, limit)
	vh.db.Offset(offset).Limit(limit).Find(&vols)
	err = vh.db.Error
	if err != nil {
		klog.V(5).Error(err, "GetVolumesWithDetail cannot get volume list from db")
		return nil, err
	}
	var vol_list []map[string]interface{}

	for _, vol := range vols {
		vol_detail := make(map[string]interface{})

		var node_ids []string
		var cpvis []ControllerPublishVolumeInfo
		vh.db.Where("vol_id = ?", vol.VolID).Find(&cpvis)
		err = vh.db.Error
		if err != nil {
			klog.V(5).Error(err, "cannot get volume published node list from db")
			return nil, err
		} else {
			for _, cpvi := range cpvis {
				node_ids = append(node_ids, cpvi.NodeID)
			}
		}

		vol_detail["published_node_ids"] = node_ids
		params := make(map[string]string)
		params[pvNameKey] = vol.PVName
		params[pvcNameKey] = vol.PVCName
		params[pvcNamespaceKey] = vol.NSName
		params[typeParameter] = "folder"
		if vol.IsBlock {
			params[typeParameter] = "disk"
		}
		vol_detail["parameters"] = params
		vol_detail["capacity"] = vol.Capacity

		vol_detail["condition_abnormal"] = true
		var fi os.FileInfo
		fi, err := os.Stat(vol.VolPath)
		if err != nil {
			vol_detail["condition_msg"] = err.Error()
		} else {
			if vol.IsBlock && fi.Size() != vol.Capacity {
				vol_detail["condition_msg"] = "file size dismatch"
			} else {
				vol_detail["condition_msg"] = "ok"
				vol_detail["condition_abnormal"] = false
			}
		}
		vol_detail["volumeId"] = vol.VolID

		vol_list = append(vol_list, vol_detail)
	}

	klog.V(5).Infof("GetVolumesWithDetail volume details obtained from %v to %v", offset, limit)
	return vol_list, nil
}

func (vh *VolumeHelper) DeleteVolume(volid string) error {
	klog.V(5).Infof("DeleteVolume try to delete volume %s", volid)
	vol, err := vh.GetVolume(volid)
	if err != nil {
		klog.V(5).Error(err, "DeleteVolume cannot get volume %s", volid)
		return err
	}

	volume_path := vol.VolPath

	tx := vh.db.Begin()
	if tx == nil || vh.db.Error != nil {
		klog.V(5).Error(err, "DeleteVolume cannot start transaction")
		return vh.db.Error
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			klog.V(5).Error(errors.New(fmt.Sprintf("%v", err)), "DeleteVolume an error accured while trans, rollback performed")
		}
	}()

	vh.db.Where("vol_id = ?", vol.VolID).Delete(&Volume{})

	symlink_dir := filepath.Join(vh.syms_path, vol.NSName)
	symlink_file := filepath.Join(symlink_dir, vol.PVCName)

	os.Remove(symlink_file)
	err = os.RemoveAll(volume_path)

	if err != nil {
		tx.Rollback()
		klog.V(5).Error(err, "DeleteVolume  volume %s cannot be deleted for %s/%s", vol.VolID, vol.NSName, vol.PVCName)
	} else {
		err = tx.Commit().Error
	}
	if err == nil {
		klog.V(5).Infof("DeleteVolume volume %s deleted for %s/%s", vol.VolID, vol.NSName, vol.PVCName)
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
	klog.V(5).Infof("ReBuildSymLinks started")

	err := os.RemoveAll(vh.syms_path)
	if err != nil {
		klog.V(5).Error(err, "ReBuildSymLinkscannot remove syms folder")
		return err
	}
	err = os.MkdirAll(vh.syms_path, 0750)
	if err != nil {
		klog.V(5).Error(err, "ReBuildSymLinkscannot recreate syms folder")
		return err
	}

	result := vh.db.Find(&vols)

	if result.Error != nil {
		klog.V(5).Error(err, "ReBuildSymLinkscannot get volumes from db")
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
		klog.V(5).Infof("ReBuildSymLinks all symlinks rebuilded")
	}

	klog.V(5).Infof("ReBuildSymLinks started")
	return err
}

func (vh *VolumeHelper) CleanUpDanglingVolumes() error {
	var vols []Volume
	klog.Infof("CleanUpDanglingVolumes started")
	vh.db.Unscoped().Where("deleted_at is not null").Find(&vols)
	err := vh.db.Error
	if err != nil {
		klog.V(5).Error(err, "CleanUpDanglingVolumes cannot get deleted volumes from db")
		return err
	}

	// Phase1 delete vols from disk if deleted from db
	for _, vol := range vols {
		volume_path := filepath.Join(vh.vols_path, vol.VolID)
		if _, err := os.Stat(volume_path); err == nil {
			err = os.RemoveAll(volume_path)
			if err != nil {
				klog.V(5).Error(err, "CleanUpDanglingVolumes cannot deleted volumes from disk")
			}
		}
	}

	// Phase2 delete vols from disk if not exists on db
	pattern := fmt.Sprintf("%s/*/*/*/*", vh.vols_path)
	fs, err := filepath.Glob(pattern)
	if err != nil {
		klog.V(5).Error(err, "CleanUpDanglingVolumes cannot read volumes (volid) from disk")
		return err
	}
	for _, f := range fs {
		var vols []Volume
		result := vh.db.Where("vol_path = ?", f).Find(&vols)
		if result.RowsAffected == 0 {
			err = os.RemoveAll(f)
			if err != nil {
				klog.V(5).Error(err, "CleanUpDanglingVolumes cannot deleted volumes from disk")
			}
		}
	}

	if err == nil {
		klog.V(5).Infof("CleanUpDanglingVolumes all dangling volumes are deleted")
	}
	// Phase3 rebuild links
	return vh.ReBuildSymLinks()
}

func (vh *VolumeHelper) Close() error {
	sqlDB, err := vh.db.DB()
	if err != nil {
		klog.V(5).Error(err, "Close error occured")
		return err
	}
	sqlDB.Close()
	return nil
}

func (vh *VolumeHelper) UpdateNodeInfoLastSeen(nodeId string, lastSeen time.Time) error {
	err := vh.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_seen"}),
	}).Create(&NodeInfo{ID: nodeId, LastSeen: lastSeen}).Error
	return err
}

func (vh *VolumeHelper) GetNodeInfo(nodeId string, age int64) (*NodeInfo, error) {
	var ni NodeInfo
	min_ls := time.Now().Add(time.Millisecond * time.Duration(age) * -1)
	result := vh.db.Where("id = ? and last_seen >= ?", nodeId, min_ls).First(&ni)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &ni, result.Error
}

func (vh *VolumeHelper) CreateControllerPublishVolumeInfo(volId, nodeId string, readonly bool) error {
	cpvi := ControllerPublishVolumeInfo{VolID: volId, NodeID: nodeId, ReadOnly: readonly}
	err := vh.db.Create(&cpvi).Error
	return err
}

func (vh *VolumeHelper) GetControllerPublishVolumeInfo(volId, nodeId string) (*ControllerPublishVolumeInfo, error) {
	var cpvi ControllerPublishVolumeInfo
	result := vh.db.Where("vol_id = ? and node_id = ?", volId, nodeId).First(&cpvi)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	return &cpvi, result.Error
}

func (vh *VolumeHelper) DeleteControllerPublishVolumeInfo(volId, nodeId string) error {
	result := vh.db.Where("vol_id = ? and node_id = ?", volId, nodeId).Delete(&ControllerPublishVolumeInfo{})
	return result.Error
}

func (vh *VolumeHelper) CreateNodePublishVolumeInfo(volId, nodeId, mountPath string, rawMount, readonly bool) error {
	npvi := NodePublishVolumeInfo{VolID: volId, NodeID: nodeId, MountPath: mountPath, RawMount: rawMount, ReadOnly: readonly}
	err := vh.db.Create(&npvi).Error
	return err
}

func (vh *VolumeHelper) DeleteNodePublishVolumeInfo(volId, nodeId, mountPath string) error {
	result := vh.db.Where("vol_id = ? and node_id = ? and mount_path = ?", volId, nodeId, mountPath).Delete(&NodePublishVolumeInfo{})
	return result.Error
}

func (vh *VolumeHelper) GetNodePublishVolumeInfo(volId, nodeId, mountPath string) (*NodePublishVolumeInfo, error) {
	var nvpi NodePublishVolumeInfo
	result := vh.db.Where("vol_id = ? and node_id = ? and mount_path = ?", volId, nodeId, mountPath).First(&nvpi)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	return &nvpi, result.Error
}

func (vol *Volume) PopulateVolumeIfRequired() (bool, error) {
	var err error
	klog.Infof("PopulateVolumeIfRequired started")
	_, err = os.Lstat(vol.VolPath)
	if os.IsNotExist(err) {
		if vol.IsBlock {
			executor := utilexec.New()
			cap_str := fmt.Sprintf("seek=%d", vol.Capacity)
			vp_str := fmt.Sprintf("of=%s", vol.VolPath)
			var output []byte
			output, err = executor.Command("dd", "if=/dev/null", "bs=1", "count=0", cap_str, vp_str).CombinedOutput()
			if err != nil {
				klog.V(5).Error(err, "PopulateVolumeIfRequired error occured %v", string(output))
				return false, err
			}
		} else {
			err := os.MkdirAll(vol.VolPath, 0750)
			if err != nil {
				klog.V(5).Error(err, "PopulateVolumeIfRequired error occured")
				return false, err
			}
		}
		klog.V(5).Infof("PopulateVolumeIfRequired volume needs populating: %s with path: %s", vol.VolID, vol.VolPath)
		return true, nil
	} else {
		if err == nil {
			klog.V(5).Infof("PopulateVolumeIfRequired volume don not need populating: %s with path: %s", vol.VolID, vol.VolPath)
			return false, nil
		}
		klog.V(5).Error(err, "PopulateVolumeIfRequired error occured")
		return false, err
	}
	return false, nil
}

func fixCapacity(capacity int64) int64 {
	if capacity < GiB {
		capacity = GiB
	}
	if capacity%MiB != 0 {
		capacity = ((capacity >> 20) + 1) << 20
	}
	return capacity
}
