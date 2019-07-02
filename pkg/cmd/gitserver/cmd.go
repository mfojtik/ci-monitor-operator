package gitserver

import (
	"github.com/spf13/cobra"

	"github.com/mfojtik/config-history-operator/pkg/gitserver"
)

func NewGitServer() *cobra.Command {
	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			gitserver.Run("/repository", "0.0.0.0:8080")
		},
	}
	cmd.Use = "gitserver"
	cmd.Short = "Start the OpenShift config history HTTP GIT server"

	return cmd
}
