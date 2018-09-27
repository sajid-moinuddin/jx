package cmd

import (
	"io"

	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1/terminal"
)

var (
	deleteAddonCloudBeesLong = templates.LongDesc(`
		Deletes the CloudBees addon
`)

	deleteAddonCloudBeesExample = templates.Examples(`
		# Deletes the CloudBees addon
		jx delete addon cloudbees
	`)
)

// DeleteAddonGiteaOptions the options for the create spring command
type DeleteAddonCDXOptions struct {
	DeleteAddonOptions

	ReleaseName string
}

// NewCmdDeleteAddonCloudBees defines the command
func NewCmdDeleteAddonCloudBees(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := &DeleteAddonGiteaOptions{
		DeleteAddonOptions: DeleteAddonOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				In:      in,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "cloudbees",
		Short:   "Deletes the CloudBees app for Kubernetes addon",
		Aliases: []string{"cloudbee", "cb", "cdx"},
		Long:    deleteAddonCloudBeesLong,
		Example: deleteAddonCloudBeesExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&options.ReleaseName, optionRelease, "r", defaultCloudBeesReleaseName, "The chart release name")
	options.addFlags(cmd)
	return cmd
}

// Run implements the command
func (o *DeleteAddonCDXOptions) Run() error {
	if o.ReleaseName == "" {
		return util.MissingOption(optionRelease)
	}
	err := o.deleteChart(o.ReleaseName, o.Purge)
	if err != nil {
		return err
	}

	return nil
}
