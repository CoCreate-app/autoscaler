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

package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/config"
	"github.com/CoCreate-app/CoCreateLB/nodeautoscaler/pkg/util"

	klog "k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	// DefaultWorkerNumber is default number of workers
	DefaultWorkerNumber = 3
)

var logger = klogr.New().WithName("kubenode-controller")

// KubeNodeController watches node resource in Kubernetes
// and trigger actions on changes
type KubeNodeController struct {
	processKeyFunc func(key string) error
	indexer        cache.Indexer
	queue          workqueue.RateLimitingInterface
	informer       cache.Controller
	context        context.Context
}

// NewController creates a new KubeNodeController
func NewController(ctx context.Context, cfg config.Config, clientset *kubernetes.Clientset, processKeyFunc func(key string) error) (*KubeNodeController, error) {
	defer klog.Flush()

	/*
		nodeListWatcher, err := createLabelSelectedListWatcher(clientset, cfg.LabelSelector)
		if err != nil {
			return nil, err
		}
	*/

	nodeListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "nodes", "", fields.Everything())

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	indexer, informer := cache.NewIndexerInformer(nodeListWatcher, &v1.Node{},
		time.Duration(cfg.CacheResyncPeriod)*time.Second, cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(obj)
				if err == nil {
					logger.V(6).Info("get added key", "key", key)
					queue.Add(key)
				}
			},
			UpdateFunc: func(old interface{}, new interface{}) {
				key, err := cache.MetaNamespaceKeyFunc(new)
				if err == nil {
					logger.V(6).Info("get updated key", "key", key)
					queue.Add(key)
				}
			},
			DeleteFunc: func(obj interface{}) {
				// IndexerInformer uses a delta queue, therefore for deletes we have to use this
				// key function.
				key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err == nil {
					logger.V(6).Info("get deleted key", "key", key)
					queue.Add(key)
				}
			},
		}, cache.Indexers{})

	return &KubeNodeController{
		processKeyFunc: processKeyFunc,
		indexer:        indexer,
		queue:          queue,
		informer:       informer,
		context:        ctx,
	}, nil
}

func createLabelSelectedListWatcher(clientset *kubernetes.Clientset, selector string) (*cache.ListWatch, error) {
	labelSelector, err := util.ParseSelector(selector)
	if err != nil {
		logger.Error(err, "failed to parse input label selector", "input label selector", selector)
		return nil, err
	}

	lw := cache.NewFilteredListWatchFromClient(clientset.CoreV1().RESTClient(), "nodes", "",
		func(options *metav1.ListOptions) {
			options.LabelSelector = labelSelector.String()
		})
	return lw, nil
}

func (c *KubeNodeController) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer c.queue.Done(key)

	// Invoke the method containing the business logic
	err := c.processKeyFunc(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleErr(err, key)
	return true
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *KubeNodeController) handleErr(err error, key interface{}) {
	defer klog.Flush()

	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < 5 {
		logger.Error(err, "error syncing node", "node key", key)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	logger.Error(err, "dropping node out of the queue", "node key", key)
}

// Run begins watching
func (c *KubeNodeController) Run(wg *sync.WaitGroup) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer c.queue.ShutDown()
	logger.Info("starting Node controller")
	wg.Add(1)
	defer wg.Done()

	go c.informer.Run(c.context.Done())

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(c.context.Done(), c.informer.HasSynced) {
		logger.Error(fmt.Errorf("timed out waiting for caches to sync"), "timed out waiting for caches to sync")
		return
	}

	for i := 0; i < DefaultWorkerNumber; i++ {
		go wait.Until(c.runWorker, time.Second, c.context.Done())
	}

	<-c.context.Done()
	logger.Info("stopping Node controller")
}

func (c *KubeNodeController) runWorker() {
	for c.processNextItem() {
	}
}

// GetByKey wraps GetByKey of internal indexer
func (c *KubeNodeController) GetByKey(key string) (interface{}, bool, error) {
	return c.indexer.GetByKey(key)
}
