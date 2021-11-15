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

package metriccalc

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	ms "github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/metricsource"
	pv "github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/provisioner"
	"github.com/go-logr/logr"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
)

const (
	scaleUpCheckPeriod = 60 * time.Second
)

var checkPeriodBackoffFactor float32 = 1.2

var logger = klogr.New().WithName("metric-calculator")

// InternalConfig is a internally used configuration
// this makes it no need to import config package, solving looping import
type InternalConfig struct {
	// ASGName is the name of auto scale group this calculator serves
	ASGName string
	// MaxBackendFailure is the maximum times of allowed provisioning failure in backend
	// only failures of scaling up are counted
	MaxBackendFailure int

	// ScaleDownThreshold indicates threshold below which scaling down might be triggered
	ScaleDownThreshold string

	// ScaleUpThreshold indicates threshold over which scaling up might be triggered
	ScaleUpThreshold string

	// AlarmWindow indicates for how long before a scale event is finally fired since it's firstly observed
	AlarmWindow int32

	// AlarmCoolDown is the minimum waiting time between 2 scaling
	AlarmCoolDown int32

	// AlarmCancelWindow denotes for how long metrics keep normal before a potential scale event is canceled
	AlarmCancelWindow int32

	// MetricsCalculatePeriod is the period metrics are calculated in
	MetricsCalculatePeriod int

	// ScaleUpTimeout denotes after how long in seconds a scaling up time out
	ScaleUpTimeout int32

	// MinNodeNum is the minimum number of existing nodes before scaling down
	MinNodeNum int

	// MaxNodeNum is the maximum number of nodes can exist after scaling up
	MaxNodeNum int
}

// NodeUpdate represents a update of nodes resource
type NodeUpdate struct {
	Key  string
	Node *v1.Node
}

// Calculator to calculate metrics and triggers scaling action
type Calculator struct {
	asgName string

	logger logr.Logger

	// the same map in autoscaler
	availNodes map[string]*v1.Node

	// number of available nodes in last calc loop
	lastNodeNum int

	// the same locker in autoscaler
	rwlock *sync.RWMutex

	// a channel receving node update from node controller
	nodeUpdateCh chan NodeUpdate

	metricSource ms.MetricSource

	// threshold for each type of metrics that should trigger scaling action
	thresholds map[ScaleT]map[ms.MetricT]float32

	// for how long before a scale event is finally fired since it's firstly observed
	alarmWindow time.Duration

	// minimum waiting time between 2 scaling
	alarmCoolDown time.Duration

	// for how long metrics keep normal before a potential scale event is canceled
	alarmCancelWindow time.Duration

	// periocially calcuate metrics in period
	calcPeriod time.Duration

	// last fired event
	lastFiredEvent *scaleEvent

	// maximum times of allowed provisioning failure in backend
	// only failures of scaling up are counted
	maxBackendFailure int

	// accumulated times of backend failure in a row
	// reset to zero for any successful scaling up
	backendFailureNum int

	// after how long in seconds a scaling up time out
	scaleUpTimeout time.Duration

	// current potential fired event
	curEvent *scaleEvent

	context context.Context

	// use to notify upper component to shutdown
	upperCloseCh chan struct{}

	// backend provisioner
	provisioner pv.Provisioner

	minNodeNum int

	maxNodeNum int
}

// NewCalculator creates a calculator instance
func NewCalculator(ctx context.Context, closeCh chan struct{}, nodeUpdateCh chan NodeUpdate,
	metricSource ms.MetricSource, p pv.Provisioner, cfg InternalConfig) (*Calculator, error) {
	ret := Calculator{
		asgName:           cfg.ASGName,
		logger:            logger.WithValues("auto scale group name", cfg.ASGName),
		availNodes:        make(map[string]*v1.Node, 4),
		rwlock:            &sync.RWMutex{},
		nodeUpdateCh:      nodeUpdateCh,
		metricSource:      metricSource,
		maxBackendFailure: cfg.MaxBackendFailure,
		context:           ctx,
		upperCloseCh:      closeCh,
		provisioner:       p,
		minNodeNum:        cfg.MinNodeNum,
		maxNodeNum:        cfg.MaxNodeNum,
	}

	sdth, err := parseThreshold(cfg.ScaleDownThreshold)
	if err != nil {
		ret.logger.Error(err, "failed to parse scale down threshold", "threshold", cfg.ScaleDownThreshold)
		return nil, err
	}

	suth, err := parseThreshold(cfg.ScaleUpThreshold)
	if err != nil {
		ret.logger.Error(err, "failed to parse scale up threshold", "threshold", cfg.ScaleUpThreshold)
		return nil, err
	}

	ret.thresholds = map[ScaleT]map[ms.MetricT]float32{
		scaleDown: sdth,
		scaleUp:   suth,
	}

	ret.alarmWindow = time.Duration(cfg.AlarmWindow) * time.Second
	ret.alarmCoolDown = time.Duration(cfg.AlarmCoolDown) * time.Second
	ret.alarmCancelWindow = time.Duration(cfg.AlarmCancelWindow) * time.Second
	ret.calcPeriod = time.Duration(cfg.MetricsCalculatePeriod) * time.Second
	ret.scaleUpTimeout = time.Duration(cfg.ScaleUpTimeout) * time.Second

	return &ret, nil
}

func parseThreshold(th string) (map[ms.MetricT]float32, error) {
	values, err := url.ParseQuery(strings.ReplaceAll(th, ",", "&"))
	if err != nil {
		return nil, err
	}
	ret := make(map[ms.MetricT]float32, 2)
	for k, v := range values {
		if strings.ToLower(k) == string(ms.MetricNodeCPU) {
			if s, err := strconv.ParseFloat(v[0], 32); err == nil {
				ret[ms.MetricNodeCPU] = float32(s)
			} else {
				return nil, err
			}
		} else if strings.ToLower(k) == string(ms.MetricNodeMem) {
			if s, err := strconv.ParseFloat(v[0], 32); err == nil {
				ret[ms.MetricNodeMem] = float32(s)
			} else {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("unkown metric type")
		}
	}
	return ret, nil
}

func (c *Calculator) handleNodeUpdate() {

	for {
		select {
		case nu := <-c.nodeUpdateCh:
			c.updateAvailNodes(nu)
		case <-c.context.Done():
			return
		}
	}
}

func (c *Calculator) updateAvailNodes(nu NodeUpdate) {
	defer klog.Flush()
	locLog := c.logger.WithValues("node key", nu.Key)
	locLog.V(3).Info("receive node update")

	var addNode, delNode, nodeReady bool

	if nu.Node == nil {
		nodeReady = false
	} else {
		nodeReady = isNodeReady(nu.Node)
	}

	addNode, delNode = c.ifUpdateNodes(nodeReady, nu.Key)

	if (!addNode) && (!delNode) {
		return
	}

	c.rwlock.Lock()
	locLog.V(4).Info("update available nodes, got write lock")
	defer c.rwlock.Unlock()

	if delNode {
		locLog.Info("remove unavailable node out of metrics calculation")
		delete(c.availNodes, nu.Key)
	}
	if addNode {
		if c.provisioner.IsManaged(nu.Key) {
			locLog.Info("add new avaialbe node into metrics calculation")
			c.availNodes[nu.Key] = nu.Node
		} else {
			locLog.Info("node is not managed by related provisioner")
		}
	}

}

func (c *Calculator) ifUpdateNodes(nodeReady bool, key string) (addNode, delNode bool) {
	addNode, delNode = false, false
	if _, ok := c.availNodes[key]; ok {
		if !nodeReady {
			delNode = true
		}
	} else if nodeReady {
		addNode = true
	}
	return addNode, delNode
}

var nodeReadyCheckMap = map[v1.NodeConditionType]v1.ConditionStatus{
	v1.NodeReady:              v1.ConditionTrue,
	v1.NodeMemoryPressure:     v1.ConditionFalse,
	v1.NodeDiskPressure:       v1.ConditionFalse,
	v1.NodePIDPressure:        v1.ConditionFalse,
	v1.NodeNetworkUnavailable: v1.ConditionFalse,
}

// node is considered as ready only when all above conditons are matched
func isNodeReady(obj *v1.Node) bool {
	for _, con := range obj.Status.Conditions {
		if st, ok := nodeReadyCheckMap[con.Type]; ok {
			if con.Status != st {
				return false
			}
		}
	}
	return true
}

// Run runs main loop calculating metrics and triggering scaling
func (c *Calculator) Run(wg *sync.WaitGroup) {
	defer klog.Flush()

	c.logger.Info("starting metric calculator")
	wg.Add(1)
	defer wg.Done()
	t := time.NewTicker(c.calcPeriod)
	defer t.Stop()

	// watch node update
	go c.handleNodeUpdate()

	var cpuUtil, memUtil float32
	var nodeNum int
	for {
		cpuUtil, memUtil, nodeNum = c.calc()
		if nodeNum > 0 {
			c.updateEvent(cpuUtil, memUtil, nodeNum)
		}

		select {
		case <-t.C:
		case <-c.context.Done():
			c.logger.Info("stopping metric calculator")
			return
		}
	}
}

func (c *Calculator) calc() (avgCPUUtil, avgMemUtil float32, effNodeNum int) {
	var cpuCap, memCap, cpuUse, memUse *resource.Quantity
	var err error
	c.rwlock.RLock()
	defer c.rwlock.RUnlock()

	for k, v := range c.availNodes {
		cpuCap = v.Status.Capacity.Cpu()
		memCap = v.Status.Capacity.Memory()
		c.logger.V(3).Info("read node's resource capacity", "node name", k, "cpu cap", cpuCap, "mem cap", memCap)

		cpuUse, err = c.metricSource.GetMetric(k, ms.MetricNodeCPU)
		if err != nil {
			c.logger.Info("failed to get metric, skip this node", "node", k, "metric", ms.MetricNodeCPU)
			continue
		}
		memUse, err = c.metricSource.GetMetric(k, ms.MetricNodeMem)
		if err != nil {
			c.logger.Info("failed to get metric, skip this node", "node", k, "metric", ms.MetricNodeMem)
			continue
		}

		c.logger.V(3).Info("read node's resource usage", "node name", k, "cpu usage", cpuUse, "mem usage", memUse)
		effNodeNum++
		avgCPUUtil += (float32(cpuUse.MilliValue()) / float32(cpuCap.MilliValue()))
		avgMemUtil += (float32(memUse.MilliValue()) / float32(memCap.MilliValue()))
	}

	c.lastNodeNum = len(c.availNodes)

	return avgCPUUtil / float32(effNodeNum), avgMemUtil / float32(effNodeNum), effNodeNum
}

func (c *Calculator) judgeEventType(cpuUtil, memUtil float32, nodeNum int) ScaleT {
	// either cpu or memory is overused, a scaling up might be needed
	if th, ok := c.thresholds[scaleUp][ms.MetricNodeCPU]; ok {
		if cpuUtil >= th {
			return scaleUp
		}
	}
	if th, ok := c.thresholds[scaleUp][ms.MetricNodeMem]; ok {
		if memUtil >= th {
			return scaleUp
		}
	}

	// only when both cpu and memory usage are below thresholds, a scaling down might be needed
	if th, ok := c.thresholds[scaleDown][ms.MetricNodeCPU]; ok {
		if cpuUtil >= th {
			return noScale
		}
	}
	if th, ok := c.thresholds[scaleDown][ms.MetricNodeMem]; ok {
		if memUtil >= th {
			return noScale
		}
	}

	return scaleDown
}

func (c *Calculator) updateEvent(cpuUtil, memUtil float32, nodeNum int) {
	c.logger.V(3).Info("evaluate alarm", "avg cpu usage", cpuUtil, "avg mem usage", memUtil)
	scaleType := c.judgeEventType(cpuUtil, memUtil, nodeNum)
	c.logger.V(3).Info("evaluated scale type", "scale type", scaleType)

	nowTime := time.Now()

	if scaleType == noScale && c.curEvent != nil {
		if c.curEvent.shouldCanceledTime.Before(nowTime) {
			c.logger.V(3).Info("cancel a scaling event", "scale type", c.curEvent.eventType,
				"event observing time", c.curEvent.firstObservedTime)
			c.curEvent = nil
		}
		return
	}

	if c.curEvent == nil || c.curEvent.eventType != scaleType {
		c.curEvent = &scaleEvent{
			firstObservedTime:  nowTime,
			shouldFiredTime:    nowTime.Add(c.alarmWindow),
			shouldCanceledTime: nowTime.Add(c.alarmCancelWindow),
			eventType:          scaleType,
		}
		return
	}
	c.logger.V(2).Info("evaluate next fire time by alarm window", "next fire time", c.curEvent.shouldFiredTime,
		"scale type", c.curEvent.eventType)
	if c.lastFiredEvent != nil {
		c.logger.V(2).Info("evaluate next fire time by alarm cool down", "next fire time", c.lastFiredEvent.allowNextFireTime,
			"scale type", c.curEvent.eventType)
	}
	if c.curEvent.shouldFiredTime.Before(nowTime) {
		if c.lastFiredEvent == nil || nowTime.After(c.lastFiredEvent.allowNextFireTime) {
			c.curEvent.firedTime = nowTime
			c.lastFiredEvent, c.curEvent = c.curEvent, nil
			// waiting for scaling finishes in backend
			// so the total waiting time between 2 scaling is scaling_time + cool_down
			c.fire(scaleType)
			c.lastFiredEvent.allowNextFireTime = time.Now().Add(c.alarmCoolDown)
			return
		}
	}

	// here means a potential scale event is still ongoing without actual firing
	c.curEvent.shouldCanceledTime = nowTime.Add(c.alarmCancelWindow)
}

// we only wait for scale up, and died if too many scaling up time out or fail
func (c *Calculator) waitForScaleUp() bool {
	stopCh := make(chan struct{})
	var curLen int
	ticker := time.NewTicker(scaleUpCheckPeriod)
	defer ticker.Stop()
	f := func() {
		c.rwlock.RLock()
		defer c.rwlock.RUnlock()
		curLen = len(c.availNodes)
		if curLen > c.lastNodeNum {
			close(stopCh)
		}
	}

	ctx, cancel := context.WithTimeout(context.TODO(), c.scaleUpTimeout)
	defer cancel()
	for {
		f()

		select {
		case <-c.context.Done():
			close(stopCh)
			return false
		case <-ticker.C:
		case <-stopCh:
			return false
		case <-ctx.Done():
			close(stopCh)
			return true
		}
	}
}

func (c *Calculator) fire(s ScaleT) {
	c.logger.Info("fire a scaling", "scale type", s)
	// only care about scale up
	if s == scaleUp {
		if c.provisioner.ScaleUp(c.maxNodeNum) {
			// maximum number of nodes allowed is reached or exceeded
			return
		}
		c.logger.Info("waiting for scaling up")
		if c.waitForScaleUp() {
			c.backendFailureNum = c.backendFailureNum + 1
			c.logger.Error(fmt.Errorf("waiting for scaling up timed out"),
				"", "timeout times", c.backendFailureNum)
			if c.backendFailureNum > c.maxBackendFailure {
				c.logger.Error(fmt.Errorf("exceed max backend failure times"),
					"stop the whole program")
				close(c.upperCloseCh)
				return
			}
		} else {
			c.backendFailureNum = 0
			c.logger.Info("finish a scale up")
		}
	} else if s == scaleDown {
		c.provisioner.ScaleDown(c.minNodeNum)
	}
}

func (c *Calculator) fakeRun(stopCh <-chan struct{}) {
	c.logger.Info("starting fake metric calculator")
	t := time.NewTicker(c.calcPeriod)
	for {
		c.fakeCalc()

		select {
		case <-t.C:
		case <-stopCh:
			c.logger.Info("stopping fake metric calculator")
			break
		}
	}
}

func (c *Calculator) fakeCalc() {
	var cpuCap, memCap string
	c.rwlock.RLock()
	defer c.rwlock.RUnlock()
	for k, v := range c.availNodes {
		cpuCap = v.Status.Capacity.Cpu().String()
		memCap = v.Status.Capacity.Memory().String()
		c.logger.Info("read node's resource capacity", "node name", k, "cpu cap", cpuCap, "mem cap", memCap)
	}
}
