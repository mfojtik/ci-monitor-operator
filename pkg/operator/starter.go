package operator

import (
	"context"
	"os"
	"time"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsv1beta1informer "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"

	"github.com/mfojtik/ci-monitor-operator/pkg/controller"
	"github.com/mfojtik/ci-monitor-operator/pkg/storage"
)

func RunOperator(ctx context.Context, controllerCtx *controllercmd.ControllerContext) error {
	kubeClient, err := apiextensionsclient.NewForConfig(controllerCtx.ProtoKubeConfig)
	if err != nil {
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(controllerCtx.KubeConfig)
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(controllerCtx.KubeConfig)
	if err != nil {
		return err
	}

	repositoryPath := "/repository"
	if repositoryPathEnv := os.Getenv("REPOSITORY_PATH"); len(repositoryPathEnv) > 0 {
		repositoryPath = repositoryPathEnv
	}
	configStore, err := storage.NewGitStorage(repositoryPath)
	if err != nil {
		return err
	}

	crdInformer := apiextensionsv1beta1informer.NewCustomResourceDefinitionInformer(kubeClient, time.Minute, cache.Indexers{})

	openshiftConfigObserver := controller.NewConfigObserverController(
		dynamicClient,
		crdInformer,
		discoveryClient,
		configStore,
		[]schema.GroupVersion{
			{
				Group:   "config.openshift.io", // Track everything under *.config.openshift.io
				Version: "v1",
			},
		},
		controllerCtx.EventRecorder,
	)

	go crdInformer.Run(ctx.Done())
	go openshiftConfigObserver.Run(ctx, 1)

	<-ctx.Done()

	return nil
}
