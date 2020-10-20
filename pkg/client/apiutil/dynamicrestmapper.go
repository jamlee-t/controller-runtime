/*
Copyright 2019 The Kubernetes Authors.

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

package apiutil

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// ErrRateLimited is returned by a RESTMapper method if the number of API
// calls has exceeded a limit within a certain time period.
type ErrRateLimited struct {
	// Duration to wait until the next API call can be made.
	Delay time.Duration
}

func (e ErrRateLimited) Error() string {
	return "too many API calls to the RESTMapper within a timeframe"
}

// DelayIfRateLimited returns the delay time until the next API call is
// allowed and true if err is of type ErrRateLimited. The zero
// time.Duration value and false are returned if err is not a ErrRateLimited.
func DelayIfRateLimited(err error) (time.Duration, bool) {
	var rlerr ErrRateLimited
	if errors.As(err, &rlerr) {
		return rlerr.Delay, true
	}
	return 0, false
}

// QUESTION(JamLee): 根据这个结构体形成 Mapper, 什么叫映射呢?
//  资源类型可由组，版本和资源（简称GVR）的元组唯一标识。同样，可以通过组，版本和种类（简称GVK）的元组唯一地标识一种种类。
//  --
//  标题：GVK 和 GVR 映射
//  GVR用于撰写REST API请求。例如，针对应用程序v1部署的REST API请求如下所示：
//  GET /apis/apps/v1/namespaces/{namespace}/deployments/{name}
//  通过读取资源的JSON或YAML，可以获得该资源的GVK。如果GVK和GVR之间存在映射，则可以发送从YAML读取的资源的REST API请求。这种映射称为REST映射。
//  使用k8s.io/client-go的dynamic client的示例 - iyacontrol的文章 - 知乎 https://zhuanlan.zhihu.com/p/165970638
//  --
//  标题：什么是 GVK 和 GVR？
//  在 Kubernetes 中要想完成一个 CRD，需要指定 group/kind 和 version，这个在 Kubernetes 的 API Server 中简称为 GVK。GVK 是定位一种类型的
//  方式，例如，daemonsets 就是 Kubernetes 中的一种资源，当我们跟 Kubernetes 说我想要创建一个 daemonsets 的时候，kubectl 是如何知道该怎么向
//  API Server 发送呢？是所有的不同资源都发向同一个 URL，还是每种资源都是不同的？
//  GVK: Group Version Kind
//  GVR: Group Resource, Kind 是对象的类型, Resource 是对象。例如 'scale', 'deployments/scale'。所以我认为 GVK 一对多 GVR
//  当我们要定义一个 GVR 的时候，那么怎么知道这个 GVR 是属于哪个 GVK 的呢？也就是前面说的，kubectl 是如何从 YAML 描述文件中知道该请求的是哪个 GVR URL？
//  这就是 REST Mapping 的功能，REST Mapping 可以指定一个 GVR（例如 daemonset 的这个例子），然后它返回对应的 GVK 以及支持的操作等。
//  例如: https://200.200.200.160:6443/apis/apps/v1/namespaces/default/deployments/mysql-exporter-prometheus-mysql-exporter/scale
// dynamicRESTMapper is a RESTMapper that dynamically discovers resource
// types at runtime.
type dynamicRESTMapper struct {
	mu           sync.RWMutex // protects the following fields
	staticMapper meta.RESTMapper
	limiter      *dynamicLimiter
	newMapper    func() (meta.RESTMapper, error)

	lazy bool
	// Used for lazy init.
	initOnce sync.Once
}

// DynamicRESTMapperOption is a functional option on the dynamicRESTMapper
type DynamicRESTMapperOption func(*dynamicRESTMapper) error

// WithLimiter sets the RESTMapper's underlying limiter to lim.
func WithLimiter(lim *rate.Limiter) DynamicRESTMapperOption {
	return func(drm *dynamicRESTMapper) error {
		drm.limiter = &dynamicLimiter{lim}
		return nil
	}
}

// WithLazyDiscovery prevents the RESTMapper from discovering REST mappings
// until an API call is made.
var WithLazyDiscovery DynamicRESTMapperOption = func(drm *dynamicRESTMapper) error {
	drm.lazy = true
	return nil
}

// WithCustomMapper supports setting a custom RESTMapper refresher instead of
// the default method, which uses a discovery client.
//
// This exists mainly for testing, but can be useful if you need tighter control
// over how discovery is performed, which discovery endpoints are queried, etc.
func WithCustomMapper(newMapper func() (meta.RESTMapper, error)) DynamicRESTMapperOption {
	return func(drm *dynamicRESTMapper) error {
		drm.newMapper = newMapper
		return nil
	}
}

// NewDynamicRESTMapper returns a dynamic RESTMapper for cfg. The dynamic
// RESTMapper dynamically discovers resource types at runtime. opts
// configure the RESTMapper.
func NewDynamicRESTMapper(cfg *rest.Config, opts ...DynamicRESTMapperOption) (meta.RESTMapper, error) {
	client, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	drm := &dynamicRESTMapper{
		limiter: &dynamicLimiter{
			rate.NewLimiter(rate.Limit(defaultRefillRate), defaultLimitSize),
		},
		newMapper: func() (meta.RESTMapper, error) {
			groupResources, err := restmapper.GetAPIGroupResources(client)
			if err != nil {
				return nil, err
			}
			return restmapper.NewDiscoveryRESTMapper(groupResources), nil
		},
	}
	for _, opt := range opts {
		if err = opt(drm); err != nil {
			return nil, err
		}
	}
	if !drm.lazy {
		if err := drm.setStaticMapper(); err != nil {
			return nil, err
		}
	}
	return drm, nil
}

var (
	// defaultRefilRate is the default rate at which potential calls are
	// added back to the "bucket" of allowed calls.
	defaultRefillRate = 5
	// defaultLimitSize is the default starting/max number of potential calls
	// per second.  Once a call is used, it's added back to the bucket at a rate
	// of defaultRefillRate per second.
	defaultLimitSize = 5
)

// setStaticMapper sets drm's staticMapper by querying its client, regardless
// of reload backoff.
func (drm *dynamicRESTMapper) setStaticMapper() error {
	newMapper, err := drm.newMapper()
	if err != nil {
		return err
	}
	drm.staticMapper = newMapper
	return nil
}

// init initializes drm only once if drm is lazy.
func (drm *dynamicRESTMapper) init() (err error) {
	drm.initOnce.Do(func() {
		if drm.lazy {
			err = drm.setStaticMapper()
		}
	})
	return err
}

// checkAndReload attempts to call the given callback, which is assumed to be dependent
// on the data in the restmapper.
//
// If the callback returns a NoKindMatchError, it will attempt to reload
// the RESTMapper's data and re-call the callback once that's occurred.
// If the callback returns any other error, the function will return immediately regardless.
//
// It will take care
// ensuring that reloads are rate-limitted and that extraneous calls aren't made.
// It's thread-safe, and worries about thread-safety for the callback (so the callback does
// not need to attempt to lock the restmapper).
func (drm *dynamicRESTMapper) checkAndReload(needsReloadErr error, checkNeedsReload func() error) error {
	// first, check the common path -- data is fresh enough
	// (use an IIFE for the lock's defer)
	err := func() error {
		drm.mu.RLock()
		defer drm.mu.RUnlock()

		return checkNeedsReload()
	}()

	// NB(directxman12): `Is` and `As` have a confusing relationship --
	// `Is` is like `== or does this implement .Is`, whereas `As` says
	// `can I type-assert into`
	needsReload := errors.As(err, &needsReloadErr)
	if !needsReload {
		return err
	}

	// if the data wasn't fresh, we'll need to try and update it, so grab the lock...
	drm.mu.Lock()
	defer drm.mu.Unlock()

	// ... and double-check that we didn't reload in the meantime
	err = checkNeedsReload()
	needsReload = errors.As(err, &needsReloadErr)
	if !needsReload {
		return err
	}

	// we're still stale, so grab a rate-limit token if we can...
	if err := drm.limiter.checkRate(); err != nil {
		return err
	}

	// ...reload...
	if err := drm.setStaticMapper(); err != nil {
		return err
	}

	// ...and return the results of the closure regardless
	return checkNeedsReload()
}

// TODO: wrap reload errors on NoKindMatchError with go 1.13 errors.

func (drm *dynamicRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	if err := drm.init(); err != nil {
		return schema.GroupVersionKind{}, err
	}
	var gvk schema.GroupVersionKind
	err := drm.checkAndReload(&meta.NoResourceMatchError{}, func() error {
		var err error
		gvk, err = drm.staticMapper.KindFor(resource)
		return err
	})
	return gvk, err
}

func (drm *dynamicRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	if err := drm.init(); err != nil {
		return nil, err
	}
	var gvks []schema.GroupVersionKind
	err := drm.checkAndReload(&meta.NoResourceMatchError{}, func() error {
		var err error
		gvks, err = drm.staticMapper.KindsFor(resource)
		return err
	})
	return gvks, err
}

func (drm *dynamicRESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	if err := drm.init(); err != nil {
		return schema.GroupVersionResource{}, err
	}

	var gvr schema.GroupVersionResource
	err := drm.checkAndReload(&meta.NoResourceMatchError{}, func() error {
		var err error
		gvr, err = drm.staticMapper.ResourceFor(input)
		return err
	})
	return gvr, err
}

func (drm *dynamicRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	if err := drm.init(); err != nil {
		return nil, err
	}
	var gvrs []schema.GroupVersionResource
	err := drm.checkAndReload(&meta.NoResourceMatchError{}, func() error {
		var err error
		gvrs, err = drm.staticMapper.ResourcesFor(input)
		return err
	})
	return gvrs, err
}

func (drm *dynamicRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	if err := drm.init(); err != nil {
		return nil, err
	}
	var mapping *meta.RESTMapping
	err := drm.checkAndReload(&meta.NoKindMatchError{}, func() error {
		var err error
		mapping, err = drm.staticMapper.RESTMapping(gk, versions...)
		return err
	})
	return mapping, err
}

func (drm *dynamicRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	if err := drm.init(); err != nil {
		return nil, err
	}
	var mappings []*meta.RESTMapping
	err := drm.checkAndReload(&meta.NoKindMatchError{}, func() error {
		var err error
		mappings, err = drm.staticMapper.RESTMappings(gk, versions...)
		return err
	})
	return mappings, err
}

func (drm *dynamicRESTMapper) ResourceSingularizer(resource string) (string, error) {
	if err := drm.init(); err != nil {
		return "", err
	}
	var singular string
	err := drm.checkAndReload(&meta.NoResourceMatchError{}, func() error {
		var err error
		singular, err = drm.staticMapper.ResourceSingularizer(resource)
		return err
	})
	return singular, err
}

// dynamicLimiter holds a rate limiter used to throttle chatty RESTMapper users.
type dynamicLimiter struct {
	*rate.Limiter
}

// checkRate returns an ErrRateLimited if too many API calls have been made
// within the set limit.
func (b *dynamicLimiter) checkRate() error {
	res := b.Reserve()
	if res.Delay() == 0 {
		return nil
	}
	res.Cancel()
	return ErrRateLimited{res.Delay()}
}
