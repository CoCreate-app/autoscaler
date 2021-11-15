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
	ms "github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/metricsource"
	pv "github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/provisioner"
)

const (
	defaultMetricSource ms.MetricSourceT = ms.MetricSourceKube
	defaultProvisioner  pv.ProvisionerT  = pv.ProvisionerRancherNodePool
)

// Config presents configuration needed
type Config struct {
	/*
	 * as autoscaler runs in and only manages a single cluster
	 * assume only need a sole kubeconfig and a sole rancher credential
	 */

	// KubeConfigFile is the path to a kubeconfig file
	// If this is empty, in-cluster config is used
	KubeConfigFile string

	// RancherUrl is the url of Rancher
	RancherURL string

	// RancherToken is used to access Rancher at RancherURL
	RancherToken string

	// RancherCA is the path to a CA to validate Rancher server
	// Insecure connection is used if this is empty
	RancherCA string

	// LeaseLockName is the lease lock resource name used for leader election
	LeaseLockName string

	// LeaseLockNamespace is the namespace where the above lease lock resource locates
	LeaseLockNamespace string

	// CacheResyncPeriod is the period in seconds in which all nodes in cache are revisited
	// to update available node list which should be considered in metrics calculation
	CacheResyncPeriod int

	// AutoScaleGroupConfig is the path to a file that claims configurations
	// of different auto scale groups
	AutoScaleGroupConfig string

	/*
	 * below parameters can be overrided by auto scale group specific configurations
	 *
	 */

	// MetricSource indicates the source where metrics are read
	MetricSource ms.MetricSourceT

	// MetricsCalculatePeriod is the period in seconds in which metrics are calculated
	MetricsCalculatePeriod int

	// LabelSelector is a list of "label_name=label_value" separated by comma.
	// autoscaler will only watch those nodes with exactlly matching label set,
	// or watch all nodes if this is set to empty.
	LabelSelector string

	// ScaleUpThreshold denotes thresholds, in ratio, exceeding which a scale up is triggered
	// for both memory and cpu metrics, e.g. memory=0.7,cpu=0.7
	// the thresholds are evaluated against average metrics value across all considered nodes
	ScaleUpThreshold string

	// ScaleDownThreshold denotes thresholds, in ratio, below which a scale down is triggered
	// for both memory and cpu metrics, e.g. memory=0.15,cpu=0.15.
	// The thresholds are evaluated against average metrics value across all considered nodes
	ScaleDownThreshold string

	// AlarmWindow specifies how long in seconds before a scaling is triggred
	// since a break of a threshold is saw
	AlarmWindow int32

	// AlarmCoolDown specifies minimum cooling down time in seconds between 2 fired scaling
	// Note that autoscaler always waiting for scaling really finishes in the backend,
	// so the actual waiting time between 2 scaling is backend_scaling_time + cool_down_time
	AlarmCoolDown int32

	// AlarmCancelWindow indicates for how long in seconds metrics keep normal before an protential
	// alarm could be canceled. This one should be larger than AlarmWindow
	AlarmCancelWindow int32

	// MaxBackendFailure spcifies maximum times of allowed provisioning failure in backend
	// only failures of scaling up are counted
	MaxBackendFailure int

	// ScaleUpTimeout indicates after how long in seconds a scaling up time out
	ScaleUpTimeout int32

	// MetricCacheExpireTime indicates for how long in seconds metrics can be read from cache since a update
	MetricCacheExpireTime int

	// BackendProvsioner indicates the type of backend used to provision nodes
	BackendProvsioner pv.ProvisionerT

	// RancherAnnotationNamespace is the name of the namespace
	// with the annotation "field.cattle.io/projectId"
	// Autoscaler will figure out current cluster ID from this annotation
	// and find node pools belonging to this cluster
	// This ensures autoscaler only manage node pools in the local cluster
	RancherAnnotationNamespace string

	// RancherNodePoolNamePrefix is the name prefix of a node pool in Rancher
	// Only nodes in this pool and match LabelSelector will be manipulate
	// Better enable related node labels in node pool level
	// This only effects when ranchernodepool is used as backend
	RancherNodePoolNamePrefix string

	// MaxNodeNum denotes at most how many nodes can exist
	// after scaling up.
	// Note that this only blocks a scaling up, rather than maintains
	// a fix number, i.e. if existing nodes are more, no scaling down
	// is triggered due to this parameter
	MaxNodeNum int

	// MinNodeNum denotes at least how many nodes must exist
	// before scaling down.
	// Note that this only blocks a scaling down, rather than maintains
	// a fix number, i.e. if existing nodes are less, no scaling up
	// is triggered due to this parameter
	MinNodeNum int
}

// NewConfig returns an empty configuration
// Do not use klogr here as klogr is not initialized yet
func NewConfig() Config {
	return Config{}
}

// Default set default values to configuration
// Do not use klogr here as klogr is not initialized yet
func Default(cfg *Config) {
	cfg.LeaseLockName = "node-autoscaler"
	cfg.LeaseLockNamespace = "node-autoscaler"
	cfg.CacheResyncPeriod = 0
	cfg.AutoScaleGroupConfig = ""
	cfg.MetricSource = defaultMetricSource
	cfg.LabelSelector = ""
	cfg.MetricsCalculatePeriod = 5
	cfg.ScaleUpThreshold = "memory=0.7,cpu=0.7"
	cfg.ScaleDownThreshold = "memory=0.15,cpu=0.15"
	cfg.AlarmWindow = 300
	cfg.AlarmCoolDown = 300
	cfg.AlarmCancelWindow = 600
	cfg.MaxBackendFailure = 3
	cfg.ScaleUpTimeout = 720
	cfg.MetricCacheExpireTime = 10
	cfg.BackendProvsioner = defaultProvisioner
	cfg.MaxNodeNum = 8
	cfg.MinNodeNum = 2
	cfg.RancherAnnotationNamespace = "cattle-system"
}
