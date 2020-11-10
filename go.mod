module github.com/kazimsarikaya/csi-sharedhostpath

go 1.15

require (
	github.com/container-storage-interface/spec v1.3.0
	github.com/google/uuid v1.1.2
	github.com/kubernetes-csi/csi-lib-utils v0.8.1
	github.com/kubernetes-csi/csi-test v2.2.0+incompatible
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.7.0
	github.com/stretchr/testify v1.5.1
	golang.org/x/net v0.0.0-20201031054903-ff519b6c9102
	golang.org/x/sys v0.0.0-20200930185726-fdedc70b468f
	google.golang.org/grpc v1.33.1
	gorm.io/driver/postgres v1.0.5
	gorm.io/gorm v1.20.5
	k8s.io/apimachinery v0.19.0
	k8s.io/klog/v2 v2.2.0
	k8s.io/mount-utils v0.20.0-beta.1
	k8s.io/utils v0.0.0-20200729134348-d5654de09c73
)
