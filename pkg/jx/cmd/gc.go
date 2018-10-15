package cmd

import (
	"io"

	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1/terminal"

	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
)

// GCOptions is the start of the data required to perform the operation.  As new fields are added, add them here instead of
// referencing the cmd.Flags()
type GCOptions struct {
	CommonOptions

	Output string
}

const (
	valid_gc_resources = `Valid resource types include:

    * activities
	* helm
	* previews
	* releases
    `
)

var (
	gc_long = templates.LongDesc(`
		Garbage collect resources

		` + valid_gc_resources + `

`)

	gc_example = templates.Examples(`
		jx gc activities
		jx gc gke
		jx gc helm
		jx gc previews
		jx gc releases

	`)
)

// NewCmdGC creates a command object for the generic "gc" action, which
// retrieves one or more resources from a server.
func NewCmdGC(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := &GCOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			In:      in,
			Out:     out,
			Err:     errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "gc TYPE [flags]",
		Short:   "Garbage collects Jenkins X resources",
		Long:    gc_long,
		Example: gc_example,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}

	cmd.AddCommand(NewCmdGCActivities(f, in, out, errOut))
	cmd.AddCommand(NewCmdGCPreviews(f, in, out, errOut))
	cmd.AddCommand(NewCmdGCGKE(f, in, out, errOut))
	cmd.AddCommand(NewCmdGCHelm(f, in, out, errOut))
	cmd.AddCommand(NewCmdGCReleases(f, in, out, errOut))

	return cmd
}

// Run implements this command
func (o *GCOptions) Run() error {
	return o.Cmd.Help()
}
