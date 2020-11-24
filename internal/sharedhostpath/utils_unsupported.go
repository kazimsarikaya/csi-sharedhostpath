// +build !linux

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
)

func getStatistics(volumePath string) (volumeStatistics, error) {
	return volumeStatistics{}, fmt.Errorf("getStatistics not supported for this build.")
}

func getBlockDeviceSize(blockDevice string) (int64, error) {
	return -1, fmt.Errorf("getBlockDeviceSize not supported for this build.")
}
