package testenv

import (
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
)

// https://cloudnative.to/kubebuilder/reference/envtest.html
func TestServer(t *testing.T) {
	//指定 testEnv 配置
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
	}
	//启动 testEnv
	testEnv.Start()
}
