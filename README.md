## config-history-operator

The purpose of this operator is to continuously collect all changes to OpenShift v4 configuration resources and record
all the changes in local Git repository. The operator offer HTTP service that can be used to clone the repository.

Each OpenShift config resource is mirrored in a local Git repository. If change is observed in the original resource
the change is applied to the local file and committed. 

As result, administrators can browse the configuration history of their OpenShift v4 cluster using Git.

##### What is being tracked?

OpenShift v4 use `*.config.openshift.io` suffix for the Custom Resource Definitions representing the cluster configuration.
Every instance of this CRD is being tracked and its changes recorded.

##### How it is tracked?

This operator run CustomResourceDefinition informer that watches all CRD in the cluster. When a CRD with `config.openshift.io`
suffix is observed, it creates a "dynamic informer" that is used to list and track any changes to all instances of this CRD.

##### Where is the GIT repository?

The default location is `/tmp/repository`. The HTTP git server is running on `0.0.0.0:8080`. To access this outside OpenShift
cluster, you can consider using Route.


##### How to deploy?

```bash
$ oc create -f ./manifests/*
```

##### How to configure?

There is no configuration currently.

##### Is this official OpenShift/RedHat supported?

Hell no. This is just a PoC (was not even tried yet ;-)
