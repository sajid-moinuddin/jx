package cmd

import (
	"io"

	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/spf13/cobra"
)

// UpgradeOptions are the flags for delete commands
type UpgradeOptions struct {
	CommonOptions
}

var (
	upgrade_long = templates.LongDesc(`
		Upgrade a the whole Jenkins-X platform.
`)

	upgrade_example = templates.Examples(`
		# upgrade the command line tools 
		jx upgrade cli

		# upgrade the platform 
		jx upgrade platform
	`)
)

// NewCmdUpgrade creates the command
func NewCmdUpgrade(f Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UpgradeOptions{
		CommonOptions{
			Factory: f,
			Out:     out,
			Err:     errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "upgrade [flags]",
		Short:   "Upgrades a resource",
		Long:    upgrade_long,
		Example: upgrade_example,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
		SuggestFor: []string{"remove", "rm"},
	}

	cmd.AddCommand(NewCmdUpgradeAddons(f, out, errOut))
	cmd.AddCommand(NewCmdUpgradeCLI(f, out, errOut))
	cmd.AddCommand(NewCmdUpgradeCluster(f, out, errOut))
	cmd.AddCommand(NewCmdUpgradeIngress(f, out, errOut))
	cmd.AddCommand(NewCmdUpgradePlatform(f, out, errOut))
	return cmd
}

// Run implements this command
func (o *UpgradeOptions) Run() error {
	return o.Cmd.Help()
}
