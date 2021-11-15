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

package autoscaler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/config"
	"github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/controller"
	mcalc "github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/metriccalc"
	ms "github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/metricsource"
	pv "github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/provisioner"
	"github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/util"

	"github.com/spf13/viper"

	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
)

var logger = klogr.New().WithName("autoscaler")

type autoScaleGroup struct {
	name string
	// metrics calculator to evaluate when to scale for this ASG
	calculator *mcalc.Calculator
	// metrics source for this ASG
	metricsource ms.MetricSource
	// provisioner for this ASG
	provisioner pv.Provisioner
	// selector for this ASG to match labels of nodes
	selector labels.Selector
	// channel to send node update to calculator
	nodeUpdateCh chan mcalc.NodeUpdate
}

// AutoScaler is the main entity managing auto scaling
type AutoScaler struct {
	mainConfig config.Config
	// controller watches all node resource in Kubernetes
	kubeNodeController *controller.KubeNodeController
	// put clientset here to support controller, general client, and leader election
	clientset *kubernetes.Clientset

	// lock used to update availNodes
	rwlock sync.RWMutex
	// auto scale groups indexed by name
	autoScaleGroups map[string]*autoScaleGroup

	context context.Context

	// context to lower components
	lowerCtx context.Context
	// cancel func for lower context
	lowerCancel context.CancelFunc
	// used by lower component for notifying shutdown
	reverseCloseCh chan struct{}
}

// NewAutoScaler creates an AutoScaler instance
func NewAutoScaler(ctx context.Context, cfg config.Config) (*AutoScaler, error) {
	as := AutoScaler{
		mainConfig:     cfg,
		rwlock:         sync.RWMutex{},
		context:        ctx,
		reverseCloseCh: make(chan struct{}),
	}

	var err error
	as.lowerCtx, as.lowerCancel = context.WithCancel(ctx)

	as.clientset, err = createKubeClient(cfg.KubeConfigFile)
	if err != nil {
		return nil, err
	}

	as.kubeNodeController, err = controller.NewController(as.lowerCtx, cfg, as.clientset, (&as).updateNodes)
	if err != nil {
		logger.Error(err, "failed to create node controller")
		return nil, err
	}

	if err = as.genAutoScaleGroups(); err != nil {
		logger.Error(err, "failed to generate auto scale groups")
		return nil, err
	}

	return &as, nil
}

// Run runs the main processing
func (a *AutoScaler) Run() {
	defer runtime.HandleCrash()
	defer klog.Flush()
	var wg sync.WaitGroup
	defer wg.Wait()

	// must be put below defer wg.Wait()
	defer func() {
		a.lowerCancel()
		for _, asg := range a.autoScaleGroups {
			close(asg.nodeUpdateCh)
		}
	}()

	logger.Info("starting auto scaler")

	go a.kubeNodeController.Run(&wg)
	for _, asg := range a.autoScaleGroups {
		go asg.calculator.Run(&wg)
	}

	select {
	case <-a.context.Done():
		close(a.reverseCloseCh)
	case <-a.reverseCloseCh:
	}
	logger.Info("stoping auto scaler")
}

func createKubeClient(kubeconfig string) (*kubernetes.Clientset, error) {
	var clientset *kubernetes.Clientset
	config, err := util.CreateRestCfg(kubeconfig)
	if err != nil {
		logger.Error(err, "failed to create Kubernetes client")
		return nil, err
	}
	if kubeconfig != "" {
		clientset, err = kubernetes.NewForConfig(config)
		if err != nil {
			logger.Error(err, "failed to create kubeclient from kubeconfig", "kubeconfig", kubeconfig)
			return nil, err
		}
	} else {
		clientset, err = kubernetes.NewForConfig(config)
		if err != nil {
			logger.Error(err, "failed to create kubeclient from in-cluster config")
			return nil, err
		}
	}

	return clientset, nil
}

func (a *AutoScaler) updateNodes(key string) error {
	defer klog.Flush()
	locLog := logger.WithValues("node key", key)

	locLog.V(4).Info("call updateNodes", "node key", key)

	var nodeObj *v1.Node
	var ok bool

	obj, exists, err := a.kubeNodeController.GetByKey(key)
	if err != nil {
		locLog.Error(err, "failed to get node object from cache by key")
		return err
	}

	if !exists {
		// since now no way to get labels, need to notify all ASG
		locLog.Info("node does not exist anymore, notify all auto scale group")
		for _, asg := range a.autoScaleGroups {
			// TODO: evaluate possibility of blocking
			select {
			case asg.nodeUpdateCh <- mcalc.NodeUpdate{Key: key, Node: nil}:
			case <-a.context.Done():
			}
		}
	} else {
		nodeObj, ok = obj.(*v1.Node)
		if !ok {
			err := fmt.Errorf("returned object is not a node")
			locLog.Error(err, "")
			return err
		}
		// match node to ASG
		for _, asg := range a.autoScaleGroups {
			match, err := util.MatchSelector(asg.selector, nodeObj)
			if err != nil {
				locLog.Error(err, "failed to match node labels")
				return err
			}
			if match {
				select {
				// TODO: evaluate possibility of blocking
				case asg.nodeUpdateCh <- mcalc.NodeUpdate{Key: key, Node: nodeObj}:
				case <-a.context.Done():
				}
			}
		}
	}
	return nil
}

func (a *AutoScaler) genAutoScaleGroups() error {
	if a.mainConfig.AutoScaleGroupConfig == "" {
		asgCfg := config.Convert(a.mainConfig, "default")
		asgCfg.Print()
		asg, err := a.genAutoScaleGroup(asgCfg)
		if err != nil {
			return err
		}
		a.autoScaleGroups = map[string]*autoScaleGroup{asg.name: asg}
		return nil
	}

	viper.SetConfigFile(a.mainConfig.AutoScaleGroupConfig)
	if err := viper.ReadInConfig(); err != nil {
		logger.Error(err, "failed to read auto scale groups config",
			"auto scale groups config file", a.mainConfig.AutoScaleGroupConfig)
		return err
	}
	asgCfgs := config.AutoScaleGroupList{}
	err := viper.Unmarshal(&asgCfgs)
	if err != nil {
		logger.Error(err, "failed to parse auto scale groups",
			"auto scale groups config file", a.mainConfig.AutoScaleGroupConfig)
		return err
	}
	a.autoScaleGroups = map[string]*autoScaleGroup{}
	for _, asgCfg := range asgCfgs.AutoScaleGroups {
		(&asgCfg).Merge(a.mainConfig)
		logger.Info("detect an auto scale group")
		(&asgCfg).Print()
		if asg, err := a.genAutoScaleGroup(asgCfg); err == nil {
			a.autoScaleGroups[asg.name] = asg
		} else {
			return err
		}
	}

	return nil
}

func (a *AutoScaler) genAutoScaleGroup(asgCfg config.AutoScaleGroup) (*autoScaleGroup, error) {
	var err error
	locLog := logger.WithValues("auto scale group name", asgCfg.Name)
	asg := &autoScaleGroup{}
	if asg.selector, err = util.ParseSelector(asgCfg.LabelSelector); err != nil {
		return nil, err
	}

	if asg.metricsource, err = createMetricSource(a.lowerCtx, a.mainConfig, asgCfg); err != nil {
		locLog.Error(err, "failed to create metrics source", "metrics source", asgCfg.MetricSource.Type)
		return nil, err
	}
	if asg.provisioner, err = createProvisioner(a.context, a.mainConfig, asgCfg, a.clientset); err != nil {
		locLog.Error(err, "failed to create backend provisioner", "backend provisioner", asgCfg.Provisioner.Type)
		return nil, err
	}
	nodeUpdateCh := make(chan mcalc.NodeUpdate, controller.DefaultWorkerNumber)
	asg.calculator, err = createMetricCalculator(a.lowerCtx, a.mainConfig, asgCfg, a.reverseCloseCh,
		nodeUpdateCh, asg.metricsource, asg.provisioner)
	if err != nil {
		locLog.Error(err, "failed to create metric calculator")
		close(nodeUpdateCh)
		return nil, err
	}
	asg.nodeUpdateCh = nodeUpdateCh
	asg.name = asgCfg.Name
	return asg, nil
}

func createMetricCalculator(ctx context.Context, cfg config.Config, asg config.AutoScaleGroup, closeCh chan struct{},
	nodeUpdateCh chan mcalc.NodeUpdate, metricSource ms.MetricSource, p pv.Provisioner) (*mcalc.Calculator, error) {
	calcCfg := mcalc.InternalConfig{
		ASGName:                asg.Name,
		MaxBackendFailure:      asg.MaxBackendFailure,
		ScaleDownThreshold:     asg.ScaleDownThreshold,
		ScaleUpThreshold:       asg.ScaleUpThreshold,
		AlarmWindow:            asg.AlarmWindow,
		AlarmCoolDown:          asg.AlarmCoolDown,
		AlarmCancelWindow:      asg.AlarmCancelWindow,
		MetricsCalculatePeriod: asg.MetricsCalculatePeriod,
		ScaleUpTimeout:         asg.ScaleUpTimeout,
		MinNodeNum:             asg.Provisioner.MinNodeNum,
		MaxNodeNum:             asg.Provisioner.MaxNodeNum,
	}
	calc, err := mcalc.NewCalculator(ctx, closeCh, nodeUpdateCh, metricSource, p, calcCfg)
	if err != nil {
		logger.Error(err, "failed to create metric calculator", "auto scale group name", asg.Name)
		return nil, err
	}
	return calc, nil
}

func createMetricSource(ctx context.Context, cfg config.Config, asg config.AutoScaleGroup) (ms.MetricSource, error) {
	switch asg.MetricSource.Type {
	case ms.MetricSourceKube:
		restCfg, err := util.CreateRestCfg(cfg.KubeConfigFile)
		if err != nil {
			return nil, err
		}
		ms, err := ms.NewKubeMetricSource(ctx, restCfg, time.Duration(asg.MetricSource.CacheExpireTime)*time.Second, asg.LabelSelector)
		if err != nil {
			return nil, err
		}
		return ms, nil
	default:
		return nil, fmt.Errorf("unkown metric source %s", cfg.MetricSource)
	}
}

func createProvisioner(pctx context.Context, cfg config.Config, asg config.AutoScaleGroup, kubeclient *kubernetes.Clientset) (pv.Provisioner, error) {
	switch asg.Provisioner.Type {
	case pv.ProvisionerRancherNodePool:
		if asg.Provisioner.RancherAnnotationNamespace == "" {
			return nil, fmt.Errorf("namespace with annotation \"%s\" must be specified", pv.RancherProjAnnotation)
		}
		if asg.Provisioner.RancherNodePoolNamePrefix == "" {
			return nil, fmt.Errorf("name of rancher node pool must be set")
		}
		proCfg := pv.InternalConfig{
			RancherURL:                 cfg.RancherURL,
			RancherToken:               cfg.RancherToken,
			RancherAnnotationNamespace: cfg.RancherAnnotationNamespace,
			RancherNodePoolNamePrefix:  asg.Provisioner.RancherNodePoolNamePrefix,
			RancherCA:                  cfg.RancherCA,
		}
		p, err := pv.NewProvisionerRancherNodePool(pctx, proCfg, kubeclient)
		if err != nil {
			return nil, err
		}
		return p, nil
	case pv.ProvisionerFake:
		return pv.NewProvisionerFake(), nil
	default:
		return nil, fmt.Errorf("unkown backend provisioner %s", cfg.BackendProvsioner)
	}
}
