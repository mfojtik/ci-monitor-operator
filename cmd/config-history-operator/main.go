package main

import (
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/client-go/pkg/version"
	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"

	"github.com/mfojtik/config-history-operator/pkg/cmd/gitserver"
	operatorcmd "github.com/mfojtik/config-history-operator/pkg/cmd/operator"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	logs.InitLogs()
	defer logs.FlushLogs()

	command := NewOperatorCommand()
	if err := command.Execute(); err != nil {
		if _, err := fmt.Fprintf(os.Stderr, "%v\n", err); err != nil {
			panic(err)
		}
		os.Exit(1)
	}
}

func NewOperatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config-history-operator",
		Short: "OpenShift configuration history operator",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				panic(err)
			}
			os.Exit(1)
		},
	}

	if v := version.Get().String(); len(v) == 0 {
		cmd.Version = "<unknown>"
	} else {
		cmd.Version = v
	}

	cmd.AddCommand(operatorcmd.NewOperator())
	cmd.AddCommand(gitserver.NewGitServer())

	return cmd
}
