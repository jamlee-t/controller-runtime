/*
Copyright 2018 The Kubernetes Authors.

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

package client

import (
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// NOTE(JamLee): 这里并不是缓存结果而是缓存 restclient。 这里会和 clientset 功能类似吗，算是对 clientset 的功能优化
// clientCache creates and caches rest clients and metadata for Kubernetes types
type clientCache struct {
	// config is the rest.Config to talk to an apiserver
	config *rest.Config

	// scheme maps go structs to GroupVersionKinds
	scheme *runtime.Scheme

	// mapper maps GroupVersionKinds to Resources
	mapper meta.RESTMapper

	// codecs are used to create a REST client for a gvk
	codecs serializer.CodecFactory

	// resourceByType caches type metadata
	resourceByType map[schema.GroupVersionKind]*resourceMeta
	mu             sync.RWMutex
}

// NOTE(JamLee): 根据 groupVersion 创建出来 Resource, 这里的 Resource其实就是 RestClient
// newResource maps obj to a Kubernetes Resource and constructs a client for that Resource.
// If the object is a list, the resource represents the item's type instead.
func (c *clientCache) newResource(gvk schema.GroupVersionKind, isList bool) (*resourceMeta, error) {
	if strings.HasSuffix(gvk.Kind, "List") && isList {
		// if this was a list, treat it as a request for the item's resource
		gvk.Kind = gvk.Kind[:len(gvk.Kind)-4]
	}

	// NOTE(JamLee): 创建client-go 的 restclient, 在这里应该能知道是如何访问 kube-apiserver
	client, err := apiutil.RESTClientForGVK(gvk, c.config, c.codecs)
	if err != nil {
		return nil, err
	}

	// NOTE(JamLee): gvk 的 Kind 和 Version 映射起来。其实就是 GVK 和 GVR 的转换。为什么不直接把 kind 首字母变小呢？mapping 其实也是
	//  一个类型
	mapping, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}

	// NOTE(JamLee): client 直接复制了嵌入值
	return &resourceMeta{Interface: client, mapping: mapping, gvk: gvk}, nil
}

// getResource returns the resource meta information for the given type of object.
// If the object is a list, the resource represents the item's type instead.
func (c *clientCache) getResource(obj runtime.Object) (*resourceMeta, error) {
	gvk, err := apiutil.GVKForObject(obj, c.scheme)
	if err != nil {
		return nil, err
	}

	// It's better to do creation work twice than to not let multiple
	// people make requests at once
	c.mu.RLock()
	r, known := c.resourceByType[gvk]
	c.mu.RUnlock()

	if known {
		return r, nil
	}

	// Initialize a new Client
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err = c.newResource(gvk, meta.IsListType(obj))
	if err != nil {
		return nil, err
	}
	c.resourceByType[gvk] = r
	return r, err
}

// NOTE(JamLee): getObjMeta 会得到一个对象。包含 type and object 信息
// getObjMeta returns objMeta containing both type and object metadata and state
func (c *clientCache) getObjMeta(obj runtime.Object) (*objMeta, error) {
	// NOTE(JamLee): 这里会获得 rest 对象 (client-go的rest对象)
	r, err := c.getResource(obj)
	if err != nil {
		return nil, err
	}
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	return &objMeta{resourceMeta: r, Object: m}, err
}

// NOTE(JamLee): 缓存时，这是被缓存对象的表示
// resourceMeta caches state for a Kubernetes type.
type resourceMeta struct {
	// NOTE(JamLee): client-go 中 RESTClient 的结构体对应的
	// client is the rest client used to talk to the apiserver
	rest.Interface
	// gvk is the GroupVersionKind of the resourceMeta
	gvk schema.GroupVersionKind
	// mapping is the rest mapping
	mapping *meta.RESTMapping
}

// isNamespaced returns true if the type is namespaced
func (r *resourceMeta) isNamespaced() bool {
	return r.mapping.Scope.Name() != meta.RESTScopeNameRoot

}

// resource returns the resource name of the type
func (r *resourceMeta) resource() string {
	return r.mapping.Resource.Resource
}

// objMeta stores type and object information about a Kubernetes type
type objMeta struct {
	// resourceMeta contains type information for the object
	*resourceMeta

	// Object contains meta data for the object instance
	// NOTE(JamLee): metadata 其实还有一个 metav1.Type 中包含 apiVersion 和 Kind
	metav1.Object
}
