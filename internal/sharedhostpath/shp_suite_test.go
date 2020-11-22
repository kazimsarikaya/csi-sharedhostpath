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
	"flag"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

func TestDriver(t *testing.T) {
	// Setup the full driver and its environment
	if *dataRoot == "" {
		t.Fatalf("dataroot parameter is empty")
	}
	if *dsn == "" {
		t.Fatalf("dns parameter is empty")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared Host Path Driver Suite")
}
