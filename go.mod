module github.com/kazimsarikaya/csi-sharedhostpath

go 1.16

require (
	github.com/container-storage-interface/spec v1.5.0
	github.com/google/uuid v1.3.0
	github.com/kubernetes-csi/csi-lib-utils v0.11.0
	github.com/kubernetes-csi/csi-test/v4 v4.3.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.18.1
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f
	golang.org/x/sys v0.0.0-20220319134239-a9b59b0215f8
	google.golang.org/grpc v1.45.0
	gorm.io/driver/postgres v1.3.1
	gorm.io/gorm v1.23.3
	k8s.io/apimachinery v0.23.5
	k8s.io/klog/v2 v2.60.1
	k8s.io/mount-utils v0.23.5
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
)
