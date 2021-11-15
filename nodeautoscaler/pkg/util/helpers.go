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

package util

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NormalizeFunc changes all flags that contain "_" separators
func NormalizeFunc(f *pflag.FlagSet, name string) pflag.NormalizedName {
	if strings.Contains(name, "_") {
		return pflag.NormalizedName(strings.Replace(name, "_", "-", -1))
	}
	return pflag.NormalizedName(name)
}

// ParseSelector parse a string like "key1=val1,key2=val2" to a label selector
func ParseSelector(selector string) (labels.Selector, error) {
	return labels.Parse(selector)
}

var _ runtime.Object = &v1.Node{}

// MatchSelector checks if given node object matches given selector
func MatchSelector(selector labels.Selector, obj *v1.Node) (bool, error) {
	accessor := meta.NewAccessor()
	l, err := accessor.Labels(obj)
	if err != nil {
		return false, err
	}
	return selector.Matches(labels.Set(l)), nil
}

// CreateRestCfg generates rest.Config based on path to kubeconfig
func CreateRestCfg(kcf string) (*rest.Config, error) {
	var config *rest.Config
	var err error
	if kcf != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kcf)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s", kcf)
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load config from in-cluster config")
		}
	}
	return config, nil
}
