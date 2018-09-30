package cmd

import (
	"io"

	"github.com/jenkins-x/jx/pkg/cloud/amazon"
	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1/terminal"
)

// GetAWSInfoOptions containers the CLI options
type GetAWSInfoOptions struct {
	GetOptions
}

var (
	getAWSInfoLong = templates.LongDesc(`
		Display the AWS information for the current user
`)

	getAWSInfoExample = templates.Examples(`
		# Get the AWS account information
		jx get aws info
	`)
)

// NewCmdGetAWSInfo creates the new command for: jx get env
func NewCmdGetAWSInfo(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := &GetAWSInfoOptions{
		GetOptions: GetOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				In:      in,
				Out:     out,
				Err:     errOut,
			},
		},
	}
	cmd := &cobra.Command{
		Use:     "aws info",
		Short:   "Displays AWS account information",
		Aliases: []string{"aws"},
		Long:    getAWSInfoLong,
		Example: getAWSInfoExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}

	options.addGetFlags(cmd)
	return cmd
}

// Run implements this command
func (o *GetAWSInfoOptions) Run() error {
	id, region, err := amazon.GetAccountIDAndRegion("", "")
	if err != nil {
		return err
	}
	log.Infof("AWS Account ID: %s\n", util.ColorInfo(id))
	log.Infof("AWS Region:     %s\n", util.ColorInfo(region))
	return nil
}
