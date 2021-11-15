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
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/rancher/norman/clientbase"
	managementClient "github.com/rancher/types/client/management/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	// RancherProjAnnotation is the annotation including
	// cluster ID and project ID in a format as "c-xxx:p-xxx"
	RancherProjAnnotation string = "field.cattle.io/projectId"
)

// InternalConfig is a config struct used internal
type InternalConfig struct {
	// RancherURL is URL of target Rancher
	RancherURL string
	// RancherToken is used to access Rancher
	RancherToken string
	// RancherAnnotationNamespace is the name of the namespace
	// with the annotation "field.cattle.io/projectId"
	RancherAnnotationNamespace string
	// RancherNodePoolNamePrefix is the name prefix of node pool which is manipulated
	RancherNodePoolNamePrefix string
	// RancherCA is used to verify Rancher server
	RancherCA string
}

type provisionerRancherNodePool struct {
	rancherURL        string
	rancherToken      string
	rancherNodePoolID string
	rancherCA         string

	logger logr.Logger

	// management client used to connect to Rancher
	rancherClient *managementClient.Client
}

// NewProvisionerRancherNodePool creates a provisionerRancherNodePool
func NewProvisionerRancherNodePool(ctx context.Context, cfg InternalConfig, kubeclient *kubernetes.Clientset) (Provisioner, error) {
	defer klog.Flush()
	if cfg.RancherAnnotationNamespace == "" || cfg.RancherNodePoolNamePrefix == "" {
		return nil, fmt.Errorf("both namespace with annotation and node pool name prefix must be set to use ranchernodepool provisioner")
	}

	p := &provisionerRancherNodePool{
		rancherURL:   cfg.RancherURL,
		rancherToken: cfg.RancherToken,
		rancherCA:    cfg.RancherCA,
	}

	err := p.createRancherClient()
	if err != nil {
		return nil, err
	}

	clusterID, err := getClusterID(ctx, cfg.RancherAnnotationNamespace, kubeclient)
	if err != nil {
		return nil, err
	}
	logger.Info("find cluster ID", "cluster ID", clusterID)

	nodePoolID := ""
	// find node pool ID
	nodePools, err := p.rancherClient.NodePool.ListAll(nil)
	if err != nil {
		logger.Error(err, "failed to list node pools in Rancher")
		return nil, err
	}
	for _, np := range nodePools.Data {
		if np.ClusterID != clusterID {
			continue
		}
		if np.HostnamePrefix == cfg.RancherNodePoolNamePrefix {
			nodePoolID = np.ID
			break
		}
	}
	if nodePoolID == "" {
		err := fmt.Errorf("can not find node pool ID")
		logger.Error(err, "", "name prefix", cfg.RancherNodePoolNamePrefix)
		return nil, err
	}
	logger.Info("find node pool", "node pool ID", nodePoolID)
	p.rancherNodePoolID = nodePoolID
	p.logger = logger.WithValues("provisioner", ProvisionerRancherNodePool, "node pool ID", nodePoolID)

	return p, nil
}

func getClusterID(pctx context.Context, name string, client *kubernetes.Clientset) (string, error) {
	ctx, cancel := context.WithTimeout(pctx, 10*time.Second)
	defer cancel()
	ns, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		logger.Error(err, "failed to get namespace", "namespace", name)
		return "", err
	}
	logger.V(5).Info("got annotations", "namespace", name, "annotations", ns.GetAnnotations())
	if proj, ok := ns.GetAnnotations()[RancherProjAnnotation]; ok {
		return strings.Split(proj, ":")[0], nil
	} else {
		err := fmt.Errorf("failed to get cluster ID from annotation")
		logger.Error(err, "", "namespace", name, "annotation", RancherProjAnnotation)
		return "", err
	}
}

func (p *provisionerRancherNodePool) createRancherClient() error {
	opts, err := p.createClientOpts()
	if err != nil {
		p.logger.Error(err, "failed to create Rancher client options")
		return err
	}

	mClient, err := managementClient.NewClient(opts)
	if err != nil {
		p.logger.Error(err, "failed to create Rancher client")
		return err
	}

	p.rancherClient = mClient

	return nil
}

func (p *provisionerRancherNodePool) createClientOpts() (*clientbase.ClientOpts, error) {
	serverURL := p.rancherURL

	if !strings.HasSuffix(serverURL, "/v3") {
		serverURL = p.rancherURL + "/v3"
	}

	var opts *clientbase.ClientOpts

	if p.rancherCA != "" {
		b, err := ioutil.ReadFile(p.rancherCA)
		if err != nil {
			p.logger.Error(err, "failed to read Rancher CA", "Rancher CA", p.rancherCA)
			return nil, err
		}
		opts = &clientbase.ClientOpts{
			URL:      serverURL,
			TokenKey: p.rancherToken,
			CACerts:  string(b),
		}
	} else {
		opts = &clientbase.ClientOpts{
			URL:      serverURL,
			TokenKey: p.rancherToken,
			Insecure: true,
		}
	}

	return opts, nil
}

func (p *provisionerRancherNodePool) Type() ProvisionerT {
	return ProvisionerRancherNodePool
}

func (p *provisionerRancherNodePool) ScaleUp(maxN int) bool {
	defer klog.Flush()
	p.logger.Info("call backend to scale up")

	nodePool, err := p.rancherClient.NodePool.ByID(p.rancherNodePoolID)
	if err != nil {
		p.logger.Error(err, "failed to get Rancher node pool", "node pool ID", p.rancherNodePoolID)
		return false
	}
	p.logger.Info("get node pool info",
		"name", nodePool.Name,
		"node labels", nodePool.NodeLabels,
		"quantity", nodePool.Quantity,
		"display name", nodePool.DisplayName)

	if nodePool.Quantity >= int64(maxN) {
		p.logger.Info("maximum allowed number of nodes reached or exceeded, ignore scaling up",
			"node pool ID", nodePool.ID, "number of existing nodes", nodePool.Quantity)
		return true
	}

	ret := nodePool.Quantity + 1
	go p.rancherClient.NodePool.Update(nodePool, map[string]int64{"quantity": ret})
	return false
}

func (p *provisionerRancherNodePool) ScaleDown(minN int) bool {
	defer klog.Flush()
	p.logger.Info("call backend to scale down")

	nodePool, err := p.rancherClient.NodePool.ByID(p.rancherNodePoolID)
	if err != nil {
		p.logger.Error(err, "failed to get Rancher node pool", "node pool ID", p.rancherNodePoolID)
		return false
	}
	p.logger.Info("get node pool info",
		"name", nodePool.Name,
		"node labels", nodePool.NodeLabels,
		"quantity", nodePool.Quantity,
		"display name", nodePool.DisplayName)

	if nodePool.Quantity <= int64(minN) {
		p.logger.Info("existing number of nodes equals or is below minimum number, ignore scaling down",
			"node pool ID", nodePool.ID, "number of existing nodes", nodePool.Quantity)
		return true
	}

	ret := nodePool.Quantity - 1
	if ret > 0 {
		go p.rancherClient.NodePool.Update(nodePool, map[string]int64{"quantity": ret})
	}
	return false
}

func (p *provisionerRancherNodePool) IsManaged(node string) bool {
	nodes, err := p.rancherClient.Node.ListAll(nil)
	if err != nil {
		p.logger.Error(err, "failed to get node list from Rancher")
		return false
	}
	for _, n := range nodes.Data {
		if n.NodePoolID != p.rancherNodePoolID {
			continue
		}
		if n.NodeName == node {
			return true
		}
	}
	return false
}
