package operator

import (
	"github.com/spf13/cobra"

	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"github.com/mfojtik/config-history-operator/pkg/operator"
	"github.com/mfojtik/config-history-operator/pkg/version"
)

func NewOperator() *cobra.Command {
	cmd := controllercmd.
		NewControllerCommandConfig("config-history-operator", version.Get(), operator.RunOperator).
		NewCommand()
	cmd.Use = "operator"
	cmd.Short = "Start the OpenShift config history operator"

	return cmd
}
