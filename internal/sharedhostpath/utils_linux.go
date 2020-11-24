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
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	klog "k8s.io/klog/v2"
	utilexec "k8s.io/utils/exec"
	"strconv"
	"strings"
)

func getStatistics(volumePath string) (volumeStatistics, error) {

	var statfs unix.Statfs_t
	err := unix.Statfs(volumePath, &statfs)
	if err != nil {
		return volumeStatistics{}, err
	}

	volStats := volumeStatistics{
		availableBytes: int64(statfs.Bavail) * int64(statfs.Bsize),
		totalBytes:     int64(statfs.Blocks) * int64(statfs.Bsize),
		usedBytes:      (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize),

		availableInodes: int64(statfs.Ffree),
		totalInodes:     int64(statfs.Files),
		usedInodes:      int64(statfs.Files) - int64(statfs.Ffree),
	}

	return volStats, nil
}

func getBlockDeviceSize(blockDevice string) (int64, error) {
	executor := utilexec.New()
	output, err := executor.Command("blockdev", "--getsize64", blockDevice).CombinedOutput()
	if err != nil {
		errstr := fmt.Sprintf("cannot get block device size: %v", err)
		klog.Errorf(errstr)
		return -1, errors.New(errstr)
	}

	strOut := strings.TrimSpace(string(output))
	gotSizeBytes, err := strconv.ParseInt(strOut, 10, 64)

	return gotSizeBytes, nil
}
