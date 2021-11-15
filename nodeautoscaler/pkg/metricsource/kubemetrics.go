/**
 * Copyright (c) 2020 CoCreate LLC
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
 * the Software, and to permit persons to whom the Software is furnished to do so,
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
 * FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
 * COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
 * IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
 * CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package metricsource

import (
	"fmt"
	"time"

	"github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/util"

	"golang.org/x/net/context"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	metricsapi "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	resourceclient "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

// use to cache metrics instead of requesting each time calculator tries to get metrics
type metricsCache struct {
	store      map[string]metricsapi.NodeMetrics
	expireTime time.Duration
	lastUpdate time.Time
	synced     bool
}

// check expiration before call this
func (c *metricsCache) getMetricByKey(key string, t MetricT) (*resource.Quantity, error) {
	if nm, ok := c.store[key]; ok {
		if t == MetricNodeCPU {
			return nm.Usage.Cpu(), nil
		}
		if t == MetricNodeMem {
			return nm.Usage.Memory(), nil
		}
	}
	return nil, fmt.Errorf("metrics for node %s is missing", key)
}

func (c *metricsCache) isExpired() bool {
	if time.Now().After(c.lastUpdate.Add(c.expireTime)) {
		return true
	}
	return false
}

func (c *metricsCache) isSynced() bool {
	return c.synced
}

func (c *metricsCache) update(store map[string]metricsapi.NodeMetrics) {
	c.store = store
	c.lastUpdate = time.Now()
	c.synced = true
}

// kubeMetricSource is an implementation of MetricSource interface
// which gets metrics via kubernetes standard metrics API
type kubeMetricSource struct {
	resourceclient.NodeMetricsInterface
	cache    *metricsCache
	context  context.Context
	selector labels.Selector
}

// NewKubeMetricSource create a new Kubernete metric source
func NewKubeMetricSource(ctx context.Context, config *rest.Config, expire time.Duration, selector string) (MetricSource, error) {
	metricsClient, err := resourceclient.NewForConfig(config)
	if err != nil {
		logger.Error(err, "failed to create metrics client")
		return nil, err
	}

	ls, err := util.ParseSelector(selector)
	if err != nil {
		logger.Error(err, "failed to parse label selector", "label selector", selector)
		return nil, err
	}

	cache := &metricsCache{expireTime: expire}
	ret := &kubeMetricSource{
		NodeMetricsInterface: metricsClient.NodeMetricses(),
		cache:                cache,
		context:              ctx,
		selector:             ls,
	}

	store, err := ret.listNoCache()
	if err != nil {
		logger.Error(err, "failed to create Kubernetes metric source")
		return nil, err
	}

	cache.update(store)

	return ret, nil
}

// Type implement Type in MetricSource interface
func (k *kubeMetricSource) Type() MetricSourceT {
	return MetricSourceKube
}

var vaildMetric = map[MetricT]interface{}{
	MetricNodeCPU: nil,
	MetricNodeMem: nil,
}

func (k *kubeMetricSource) validMetricT(t MetricT) bool {
	_, ok := vaildMetric[t]
	return ok
}

func (k *kubeMetricSource) listNoCache() (map[string]metricsapi.NodeMetrics, error) {
	nml, err := k.List(k.context, metav1.ListOptions{LabelSelector: k.selector.String()})
	if err != nil {
		logger.Error(err, "failed to list node metrics with label selector", "label selector", k.selector)
		return nil, err
	}

	ret := map[string]metricsapi.NodeMetrics{}
	for _, nm := range (*nml).Items {
		ret[nm.Name] = nm
	}
	return ret, nil
}

func (k *kubeMetricSource) GetMetric(key string, t MetricT) (*resource.Quantity, error) {

	if k.cache.isExpired() || !k.cache.isSynced() {
		store, err := k.listNoCache()
		if err != nil {
			return nil, err
		}
		k.cache.update(store)
	}

	return k.cache.getMetricByKey(key, t)
}
