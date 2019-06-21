package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsv1beta1informer "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1beta1"
	apiextensionsv1beta1lister "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/mfojtik/config-history-operator/pkg/storage"
)

const (
	configSuffix           = ".config.openshift.io"
	controllerWorkQueueKey = "key"
)

var defaultResyncDuration = 5 * time.Minute

type ConfigObserverController struct {
	cachesToSync []cache.InformerSynced
	queue        workqueue.RateLimitingInterface
	recorder     events.Recorder
	stopCh       <-chan struct{}

	crdLister       apiextensionsv1beta1lister.CustomResourceDefinitionLister
	crdInformer     cache.SharedIndexInformer
	dynamicClient   dynamic.Interface
	cachedDiscovery discovery.CachedDiscoveryInterface

	openshiftConfigObservers []*dynamicConfigInformer
	configStorage            storage.ConfigStorage
}

func NewOpenShiftConfigObserverController(
	dynamicClient dynamic.Interface,
	extensionsClient apiextensionsclient.Interface,
	discoveryClient *discovery.DiscoveryClient,
	configStorage storage.ConfigStorage,
) (*ConfigObserverController, error) {
	c := &ConfigObserverController{
		dynamicClient: dynamicClient,
		queue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ConfigObserverController"),
		crdInformer:   apiextensionsv1beta1informer.NewCustomResourceDefinitionInformer(extensionsClient, defaultResyncDuration, cache.Indexers{}),
		configStorage: configStorage,
	}

	c.cachedDiscovery = memory.NewMemCacheClient(discoveryClient)
	c.crdLister = apiextensionsv1beta1lister.NewCustomResourceDefinitionLister(c.crdInformer.GetIndexer())
	c.crdInformer.AddEventHandler(c.eventHandler())
	c.cachesToSync = append(c.cachesToSync, c.crdInformer.HasSynced)

	return c, nil
}

// currentOpenShiftConfigResourceKinds returns list of group version configKind for OpenShift configuration types.
func (c *ConfigObserverController) currentOpenShiftConfigResourceKinds() ([]schema.GroupVersionKind, error) {
	observedCrds, err := c.crdLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var currentConfigResources []schema.GroupVersionKind
	for _, crd := range observedCrds {
		// Match only .config.openshift.io
		if !strings.HasSuffix(crd.GetName(), configSuffix) {
			continue
		}
		for _, version := range crd.Spec.Versions {
			if !version.Served {
				continue
			}
			currentConfigResources = append(currentConfigResources, schema.GroupVersionKind{
				Group:   crd.Name,
				Version: version.Name,
				Kind:    crd.Spec.Names.Kind,
			})
		}
	}
	return currentConfigResources, nil
}

func (c *ConfigObserverController) sync() error {
	current, err := c.currentOpenShiftConfigResourceKinds()
	if err != nil {
		return err
	}

	// TODO: The CRD delete case is not handled
	var kindNeedObserver []schema.GroupVersionKind
	for _, configKind := range current {
		hasObserver := false
		for _, o := range c.openshiftConfigObservers {
			if o.isKind(configKind) {
				hasObserver = true
				break
			}
		}
		if !hasObserver {
			kindNeedObserver = append(kindNeedObserver, configKind)
		}
	}

	var waitForCacheSyncFn []cache.InformerSynced

	// If we have new CRD refresh the discovery info and update the mapper
	if len(kindNeedObserver) > 0 {
		// NOTE: this is very time expensive, only do this when we have new kinds
		c.cachedDiscovery.Invalidate()
		gr, err := restmapper.GetAPIGroupResources(c.cachedDiscovery)
		if err != nil {
			return err
		}

		mapper := restmapper.NewDiscoveryRESTMapper(gr)
		for _, kind := range kindNeedObserver {
			mapping, err := mapper.RESTMapping(kind.GroupKind(), kind.Version)
			if err != nil {
				// better luck next time
				continue
			}

			// we got mapping, lets run the dynamicInformer for the config and install GIT configStorage event handlers
			dynamicInformer := newDynamicConfigInformer(kind.Kind, mapping.Resource, c.dynamicClient, c.configStorage.EventHandlers())
			waitForCacheSyncFn = append(waitForCacheSyncFn, dynamicInformer.hasInformerCacheSynced)

			go func() {
				dynamicInformer.run(c.stopCh)
				klog.Infof("Started %s", dynamicInformer)
			}()
			c.openshiftConfigObservers = append(c.openshiftConfigObservers, dynamicInformer)
		}
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	if !cache.WaitForCacheSync(ctx.Done(), waitForCacheSyncFn...) {
		return fmt.Errorf("timeout while waiting for dynamic informers to start: %#v", kindNeedObserver)
	}

	return nil
}

// eventHandler queues the operator to check spec and status
func (c *ConfigObserverController) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.queue.Add(controllerWorkQueueKey) },
		UpdateFunc: func(old, new interface{}) { c.queue.Add(controllerWorkQueueKey) },
		DeleteFunc: func(obj interface{}) { c.queue.Add(controllerWorkQueueKey) },
	}
}

// Run starts the kube-apiserver and blocks until stopCh is closed.
func (c *ConfigObserverController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	// Passed to individual dynamic informers
	c.stopCh = stopCh

	klog.Infof("Starting ConfigObserver")
	defer klog.Infof("Shutting down ConfigObserver")

	if !cache.WaitForCacheSync(stopCh, c.cachesToSync...) {
		return
	}

	// doesn't matter what workers say, only start one.
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
}

func (c *ConfigObserverController) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *ConfigObserverController) processNextWorkItem() bool {
	dsKey, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(dsKey)

	err := c.sync()
	if err == nil {
		c.queue.Forget(dsKey)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with : %v", dsKey, err))
	c.queue.AddRateLimited(dsKey)

	return true
}
