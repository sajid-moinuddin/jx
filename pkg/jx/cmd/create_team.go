package cmd

import (
	"fmt"
	"io"

	"github.com/jenkins-x/jx/pkg/kube"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1/terminal"

	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
)

var (
	createTeamLong = templates.LongDesc(`
		Creates a Team
`)

	createTeamExample = templates.Examples(`
		# Create a new pending Team which can then be provisioned
		jx create team myname
"
	`)
)

// CreateTeamOptions the options for the create spring command
type CreateTeamOptions struct {
	CreateOptions

	Name    string
	Members []string
}

// NewCmdCreateTeam creates a command object for the "create" command
func NewCmdCreateTeam(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := &CreateTeamOptions{
		CreateOptions: CreateOptions{
			CommonOptions: CommonOptions{
				Factory: f,
				In:      in,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "team",
		Short:   "Create a new Team which is then provisioned later on",
		Aliases: []string{"env"},
		Long:    createTeamLong,
		Example: createTeamExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}

	cmd.Flags().StringVarP(&options.Name, optionName, "n", "", "The name of the new Team. Should be all lower case and no special characters other than '-'")
	cmd.Flags().StringArrayVarP(&options.Members, "member", "m", []string{}, "The usernames of the members to add to the Team")

	options.addCommonFlags(cmd)
	return cmd
}

// Run implements the command
func (o *CreateTeamOptions) Run() error {
	kubeClient, _, err := o.KubeClient()
	if err != nil {
		return err
	}
	err = o.registerTeamCRD()
	if err != nil {
		return err
	}

	jxClient, devNs, err := o.JXClientAndDevNamespace()
	if err != nil {
		return err
	}

	ns, err := kube.GetAdminNamespace(kubeClient, devNs)
	if err != nil {
		return err
	}

	_, names, err := kube.GetPendingTeams(jxClient, ns)
	if err != nil {
		return err
	}

	name := o.Name
	if name == "" {
		args := o.Args
		if len(args) > 0 {
			name = args[0]
		}
	}
	if name == "" {
		return util.MissingOption(optionName)
	}

	if util.StringArrayIndex(names, name) >= 0 {
		return fmt.Errorf("The Team %s already exists!", name)
	}

	// TODO configure other properties?
	team := kube.CreateTeam(ns, name, o.Members)
	_, err = jxClient.JenkinsV1().Teams(ns).Create(team)
	if err != nil {
		return fmt.Errorf("Failed to create Team %s: %s", name, err)
	}
	log.Infof("Created Team: %s\n", util.ColorInfo(name))
	return nil
}
