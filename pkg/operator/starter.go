package operator

import (
	"context"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

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

	// TODO: Make this configurable
	configStore, err := storage.NewGitStorage("/repository")
	if err != nil {
		return err
	}

	openshiftConfigObserver := controller.NewConfigObserverController(
		dynamicClient,
		kubeClient,
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

	go openshiftConfigObserver.Run(ctx, 1)

	<-ctx.Done()

	return nil
}
