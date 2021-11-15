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

package config

import (
	"fmt"

	ms "github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/metricsource"
	pv "github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/provisioner"
)

// AutoScaleGroupList represents a list of ASG
type AutoScaleGroupList struct {
	AutoScaleGroups []AutoScaleGroup `mapstructure:"autoScaleGroups"`
}

// AutoScaleGroup represents a group of nodes with same labels
// and/or within same node pool if applicable,
// which auto scale upon same thresholds.
// Different AutoScaleGroup can have different minium/maximum
// number of nodes
type AutoScaleGroup struct {
	// Name is the unique name of ASG
	Name string `mapstructure:"name"`
	// MetricsCalculatePeriod is the period in seconds in which metrics are calculated
	MetricsCalculatePeriod int `mapstructure:"metricCalculatePeriod"`
	// ScaleUpThreshold indicates thresholds
	// exceeding any of which a scaling up may happen.
	// All thresholds must be indicated as a ratio of total
	// available divided by current usage.
	// Metrics are measured in average across all nodes.
	// Currently only cpu and memory are supported.
	// Example: "cpu=0.7,memory=0.7"
	ScaleUpThreshold string `mapstructure:"scaleUpThreshold"`
	// ScaleDownThreshold indicates thresholds that
	// a scaling down may happen when all metrics must fall below thresholds
	ScaleDownThreshold string `mapstructure:"scaleDownThreshold"`
	// LabelSelector is a set of labels all nodes in this ASG
	// must have.
	// Example: "key1=value1,key2=value2"
	LabelSelector string `mapstructure:"labelSelector"`
	// MestricSource is the metrics source used by this ASG.
	MetricSource MetricSource `mapstructure:"metricSource"`
	// Provisioner is the backend nodes provisioner used by this ASG.
	Provisioner Provisioner `mapstructure:"provisioner"`
	// AlarmWindow specifies how long in seconds before a scaling is triggred
	// since a break of a threshold is saw
	AlarmWindow int32 `mapstructure:"alarmWindow"`
	// AlarmCoolDown specifies minimum cooling down time in seconds between 2 fired scaling
	// Note that autoscaler always waiting for scaling really finishes in the backend,
	// so the actual waiting time between 2 scaling is backend_scaling_time + cool_down_time
	AlarmCoolDown int32 `mapstructure:"alarmCoolDown"`
	// AlarmCancelWindow indicates for how long in seconds metrics keep normal before an protential
	// alarm could be canceled. This one should be larger than AlarmWindow
	AlarmCancelWindow int32 `mapstructure:"alarmCancelWindow"`
	// MaxBackendFailure spcifies maximum times of allowed provisioning failure in backend
	// only failures of scaling up are counted
	MaxBackendFailure int `mapstructure:"maxBackendFailure"`
	// ScaleUpTimeout indicates after how long in seconds a scaling up time out
	ScaleUpTimeout int32 `masptructure:"scaleUpTimeout"`
}

// MestricSource represents a type of metrics source.
type MetricSource struct {
	// Type of metrics source, currently only "kubernetes" type is supported
	// which uses Kubernetes metrics API
	Type ms.MetricSourceT `mapstructure:"type"`
	// MetricCacheExpireTime indicates for how long in seconds metrics
	// can be read from cache since a update
	CacheExpireTime int `mapstructure:"cacheExpireTime"`
}

// Provisioner represents a type of backend provisioner
type Provisioner struct {
	// Type of provisioner, currently only "ranchernodepool" is supported
	// which uses Rancher's node pool API for node provisioning.
	// Note that when use "ranchernodepool", NodePoolID must be specified,
	// and the LabelSelector should match all the nodes in this pool
	Type pv.ProvisionerT `mapstructure:"type"`
	// MaxNodeNum denotes at most how many nodes can exist
	// after scaling up.
	// Note that this only blocks a scaling up, rather than maintains
	// a fix number, i.e. if existing nodes are more, no scaling down
	// is triggered due to this parameter
	MaxNodeNum int `mapstructure:"maxNodeNum"`
	// MinNodeNum denotes at least how many nodes must exist
	// before scaling down.
	// Note that this only blocks a scaling down, rather than maintains
	// a fix number, i.e. if existing nodes are less, no scaling up
	// is triggered due to this parameter
	MinNodeNum int `mapstructure:"minNodeNum"`

	// RancherAnnotationNamespace is the name of the namespace
	// with the annotation "field.cattle.io/projectId"
	// Autoscaler will figure out current cluster ID from this annotation
	// and find node pools belonging to this cluster
	// This ensures autoscaler only manage node pools in the local cluster
	RancherAnnotationNamespace string `mapstructure:"rancherAnnotationNamespace,omitempty"`

	// RancherNodePoolNamePrefix is the name prefix of a node pool in Rancher
	// Only nodes in this pool and match LabelSelector will be manipulate
	// Better enable related node labels in node pool level
	// This only effects when ranchernodepool is used as backend
	RancherNodePoolNamePrefix string `mapstructure:"rancherNodePoolNamePrefix,omitempty"`
}

// Print prints out parameters in ASG
func (a *AutoScaleGroup) Print() {
	fmt.Println("name: ", a.Name)
	fmt.Println("metricsCalculatePeriod: ", a.MetricsCalculatePeriod)
	fmt.Println("scaleUpThreshold: ", a.ScaleUpThreshold)
	fmt.Println("scaleDownThreshold: ", a.ScaleDownThreshold)
	fmt.Println("labelSelector: ", a.LabelSelector)
	fmt.Println("alarmWindow: ", a.AlarmWindow)
	fmt.Println("alarmCoolDown: ", a.AlarmCoolDown)
	fmt.Println("alarmCancelWindow: ", a.AlarmCancelWindow)
	fmt.Println("maxBackendFailure: ", a.MaxBackendFailure)
	fmt.Println("scaleUpTimeout: ", a.ScaleUpTimeout)
	fmt.Println("metricSourceType: ", a.MetricSource.Type)
	fmt.Println("metricCacheExpireTime: ", a.MetricSource.CacheExpireTime)
	fmt.Println("provisionerType: ", a.Provisioner.Type)
	if a.Provisioner.Type == pv.ProvisionerRancherNodePool {
		fmt.Println("rancherAnnotationNamespace: ", a.Provisioner.RancherAnnotationNamespace)
		fmt.Println("rancherNodePoolNamePrefix: ", a.Provisioner.RancherNodePoolNamePrefix)
	}
	fmt.Println("maxNodeNum: ", a.Provisioner.MaxNodeNum)
	fmt.Println("minNodeNum: ", a.Provisioner.MinNodeNum)
}

// Merge merges parameters from main config
func (a *AutoScaleGroup) Merge(cfg Config) {
	if a.MetricsCalculatePeriod == 0 {
		a.MetricsCalculatePeriod = cfg.MetricsCalculatePeriod
	}
	if a.ScaleUpThreshold == "" {
		a.ScaleUpThreshold = cfg.ScaleUpThreshold
	}
	if a.ScaleDownThreshold == "" {
		a.ScaleDownThreshold = cfg.ScaleDownThreshold
	}
	if a.LabelSelector == "" {
		a.LabelSelector = cfg.LabelSelector
	}
	if a.AlarmWindow == 0 {
		a.AlarmWindow = cfg.AlarmWindow
	}
	if a.AlarmCoolDown == 0 {
		a.AlarmCoolDown = cfg.AlarmCoolDown
	}
	if a.AlarmCancelWindow == 0 {
		a.AlarmCancelWindow = cfg.AlarmCancelWindow
	}
	if a.MaxBackendFailure == 0 {
		a.MaxBackendFailure = cfg.MaxBackendFailure
	}
	if a.ScaleUpTimeout == 0 {
		a.ScaleUpTimeout = cfg.ScaleUpTimeout
	}
	if a.MetricSource.CacheExpireTime == 0 {
		a.MetricSource.CacheExpireTime = cfg.MetricCacheExpireTime
	}
	if a.Provisioner.MaxNodeNum == 0 {
		a.Provisioner.MaxNodeNum = cfg.MaxNodeNum
	}
	if a.Provisioner.MinNodeNum == 0 {
		a.Provisioner.MinNodeNum = cfg.MinNodeNum
	}
	if a.Provisioner.Type == pv.ProvisionerRancherNodePool {
		if a.Provisioner.RancherAnnotationNamespace == "" {
			a.Provisioner.RancherAnnotationNamespace = cfg.RancherAnnotationNamespace
		}
		if a.Provisioner.RancherNodePoolNamePrefix == "" {
			a.Provisioner.RancherNodePoolNamePrefix = cfg.RancherNodePoolNamePrefix
		}
	}
}

// Convert generates an AutoScaleGroup with given name
// on values from main config
func Convert(cfg Config, name string) AutoScaleGroup {
	return AutoScaleGroup{
		Name:                   name,
		MetricsCalculatePeriod: cfg.MetricsCalculatePeriod,
		ScaleUpThreshold:       cfg.ScaleUpThreshold,
		ScaleDownThreshold:     cfg.ScaleDownThreshold,
		LabelSelector:          cfg.LabelSelector,
		MetricSource: MetricSource{
			Type:            cfg.MetricSource,
			CacheExpireTime: cfg.MetricCacheExpireTime,
		},
		Provisioner: Provisioner{
			Type:                       cfg.BackendProvsioner,
			MaxNodeNum:                 cfg.MaxNodeNum,
			MinNodeNum:                 cfg.MinNodeNum,
			RancherAnnotationNamespace: cfg.RancherAnnotationNamespace,
			RancherNodePoolNamePrefix:  cfg.RancherNodePoolNamePrefix,
		},
		AlarmWindow:       cfg.AlarmWindow,
		AlarmCoolDown:     cfg.AlarmCoolDown,
		AlarmCancelWindow: cfg.AlarmCancelWindow,
		MaxBackendFailure: cfg.MaxBackendFailure,
		ScaleUpTimeout:    cfg.ScaleUpTimeout,
	}
}
