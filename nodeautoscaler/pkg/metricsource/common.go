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

	"k8s.io/klog/v2/klogr"

	"k8s.io/apimachinery/pkg/api/resource"
)

var logger = klogr.New().WithName("metric-srouce")

// MetricSourceT indicates the type of metric source
type MetricSourceT string

const (
	// MetricSourceKube means use Kubernetes metrics API as the source
	MetricSourceKube MetricSourceT = "kubernetes"
)

// SupportSourceType returns the list of supporting types of metric source
// Update this func when new type is added
func SupportSourceType() string {
	return fmt.Sprintf("\"%s\"", MetricSourceKube)
}

// MetricSource is the interface for retrieving metrics
type MetricSource interface {
	Type() MetricSourceT
	// GetMetric returns resource usage of specific type of an entity identified by a string
	// Note that here we use Quantity defined in Kubernetes API
	// as the unified scalable value for resource usage, where:
	// cpu usage is measured in cores, like 500m = .5 cores
	// memory usage is measured in bytes, like 500Gi = 500GiB = 500 * 1024 * 1024 * 1024
	GetMetric(string, MetricT) (*resource.Quantity, error)
}

// MetricT is the type of metric a metric source supports
type MetricT string

const (
	// MetricNodeMem indicates node's memory usage
	MetricNodeMem MetricT = "memory"
	// MetricNodeCPU indicates node's cpu usage
	MetricNodeCPU MetricT = "cpu"
)
