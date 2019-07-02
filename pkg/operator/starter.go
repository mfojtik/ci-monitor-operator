package operator

import (
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog"

	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"github.com/mfojtik/config-history-operator/pkg/controller"
	"github.com/mfojtik/config-history-operator/pkg/storage"
)

func RunOperator(ctx *controllercmd.ControllerContext) error {
	kubeClient, err := apiextensionsclient.NewForConfig(ctx.ProtoKubeConfig)
	if err != nil {
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(ctx.KubeConfig)
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(ctx.KubeConfig)
	if err != nil {
		return err
	}

	configStore, err := storage.NewGitStorage("/repository")
	if err != nil {
		return err
	}

	openshiftConfigObserver, err := controller.NewOpenShiftConfigObserverController(dynamicClient, kubeClient, discoveryClient, configStore)
	if err != nil {
		return err
	}

	go openshiftConfigObserver.Run(ctx.Done())

	<-ctx.Done()

	klog.Warningf("operator stopped")
	return nil
}
