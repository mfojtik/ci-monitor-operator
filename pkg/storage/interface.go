package storage

import (
	"k8s.io/client-go/tools/cache"
)

type ConfigStorage interface {
	// The only job for the storage is to provide event handlers to react on configuration changes
	EventHandlers() cache.ResourceEventHandlerFuncs
}
