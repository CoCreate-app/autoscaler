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

package provisioner

import (
	"fmt"

	"k8s.io/klog/v2/klogr"
)

var logger = klogr.New().WithName("provisioner")

// ProvisionerT indicates the type of provisioner
type ProvisionerT string

const (
	// ProvisionerRancherNodePool means utilizing Rancher's node pool API to provision nodes
	ProvisionerRancherNodePool ProvisionerT = "ranchernodepool"
	// ProvisionerFake is used to test
	ProvisionerFake ProvisionerT = "fake"
)

// SupportProvisionerType returns the list of supporting types of provisioner
// Update this func when new type is added
func SupportProvisionerType() string {
	return fmt.Sprintf("\"%s\", \"%s\"", ProvisionerRancherNodePool, ProvisionerFake)
}

// Provisioner is the interface for provisioning nodes
type Provisioner interface {
	Type() ProvisionerT
	// ScaleUp calls backend system to scale up ONE node
	// with the maximum number of nodes can exist after scaling up.
	// This func returns a bool tells caller whether current number
	// of exiting nodes reaches or exceeds maximum number of nodes
	// Note that this func calls backend in an async manner,
	// caller should not rely on any response
	// from backend
	ScaleUp(int) bool
	// ScaleDown calls backend system to scale down ONE node
	// with the minimum number of nodes existing before scaling down.
	// This func returns a bool tells caller whether current number
	// of exiting nodes equals or is less than minimum number of nodes
	// Note that this func calls backend in an async manner,
	// caller should not rely on any response
	// from backend
	ScaleDown(int) bool
	// IsManaged checks if the node can be managed by the provisioner
	// which is given by the node name
	IsManaged(string) bool
}
