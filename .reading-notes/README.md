# Controller Runtime 

## 概念
cache: 本质上其实是 informerCache。  
restmapper: gvk 到 gvr 的映射。

`Manager` 组织了 `Controller`, `Controller` 组织了 `Reconciler`, `Source` 。 `Source` 中包含了 `cache`。 
`controller-runtime` 中重新实现了 `client`. `cache` 被设置到 `client` 中。 `client` 被设置到 manager 中，多个 controller 共享这个 client。 

client 包含的关系:  
```
client > informer > listwatch
```
client 和 cache 本身没有关系的。但是 client 包中的 `DelegatingClient` 用到了 cache. cache 实现了 client 包中的 reader。所以 cache 是一个reader， 此外它也是一个informer管理者。

```go
// pkg/manager/manager.go:411
return &client.DelegatingClient{
    Reader: &client.DelegatingReader{
        // NOTE(JamLee): informerCache，处理结构化和非结构化。
        CacheReader:  cache,
        ClientReader: c,
    },
    Writer:       c,
    StatusClient: c,
}, nil
```

runtime.Object 的秘密在于：
```go
// /Users/jamlee/go/pkg/mod/k8s.io/apimachinery@v0.18.6/pkg/apis/meta/v1/meta.go:119
func (obj *TypeMeta) GetObjectKind() schema.ObjectKind { return obj }
```
系统里有 Config, ApiReader, Scheme, RestMapper, Cache, StopChannel, Logger, Injector 这些核心的对象。

## client 包
client 封装 client-go 的 rest.RESTClient. 底层还是用这个 rest.RESTClient 发送的请求。

## 单元测试
Ginkgo是一个BDD风格的Go测试框架，旨在帮助你有效地编写富有表现力的全方位测试。它最好与Gomega匹配器库配对使用，但它的设计是与匹配器无关的。
```shell
# 创建 suite
ginkgo bootstrap
# suite 中添加 spec
ginkgo generate book
```
查看 ginkgo 文件夹中例子.  

单元测试启动了etcd 和 apiserver 来模拟测试，手动启动 kube-apiserver 也是可以的.
```shell
/usr/local/kubebuilder/bin/etcd --listen-peer-urls=http://localhost:0 --advertise-client-urls=http://127.0.0.1:65416 --listen-client-urls=http://127.0.0.1:65416 --data-dir=/var/folders/dm/_5krfvmx5m71fhhxwylzqgn40000gn/T/k8s_test_framework_998052184
usr/local/kubebuilder/bin/kube-apiserver --advertise-address=127.0.0.1 --etcd-servers=http://127.0.0.1:65416 --cert-dir=/var/folders/dm/_5krfvmx5m71fhhxwylzqgn40000gn/T/k8s_test_framework_899819223 --insecure-port=65428 --insecure-bind-address=127.0.0.1 --secure-port=65429 --disable-admission-plugins=ServiceAccount --service-cluster-ip-range=10.0.0.0/24 --allow-privileged=true
```
查看 testenv 文件夹中例子.

其他：
二进制安装高可用 k8s
https://juejin.im/post/6844904205556121608

## 奇怪的包
schema
貌似只是定义了小小的结构体，例如 GroupVersion 
jetbrains://goland/navigate/reference?project=controller-runtime&path=~/go/pkg/mod/k8s.io/apimachinery@v0.18.6/pkg/runtime/schema