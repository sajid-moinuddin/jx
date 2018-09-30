package cmd

import (
	"fmt"
	"io"

	"strings"

	"github.com/jenkins-x/jx/pkg/auth"
	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/jenkins-x/jx/pkg/kube"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1/terminal"
)

var (
	delete_jenkins_user_long = templates.LongDesc(`
		Deletes one or more Jenkins user tokens from your local settings
`)

	delete_jenkins_user_example = templates.Examples(`
		# Deletes the current Jenkins token
		jx delete jenkins user admin
	`)
)

// DeleteJenkinsUserOptions the options for the create spring command
type DeleteJenkinsUserOptions struct {
	CreateOptions

	ServerFlags ServerFlags
}

// NewCmdDeleteJenkinsUser defines the command
func NewCmdDeleteJenkinsUser(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := &DeleteJenkinsUserOptions{
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
		Use:     "user",
		Short:   "Deletes one or more Jenkins user API tokens",
		Aliases: []string{"token"},
		Long:    delete_jenkins_user_long,
		Example: delete_jenkins_user_example,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}
	options.ServerFlags.addGitServerFlags(cmd)
	return cmd
}

// Run implements the command
func (o *DeleteJenkinsUserOptions) Run() error {
	args := o.Args
	if len(args) == 0 {
		return fmt.Errorf("Missing Jenkins user name")
	}
	kubeClient, ns, err := o.KubeClient()
	if err != nil {
		return err
	}
	authConfigSvc, err := o.Factory.CreateJenkinsAuthConfigService(kubeClient, ns)
	if err != nil {
		return err
	}
	config := authConfigSvc.Config()

	var server *auth.AuthServer
	if o.ServerFlags.IsEmpty() {
		url := ""
		url, err = o.findService(kube.ServiceJenkins)
		if err != nil {
			return err
		}
		server = config.GetOrCreateServer(url)
	} else {
		server, err = o.findServer(config, &o.ServerFlags, "jenkins server", "Try installing one via: jx create team", false)
		if err != nil {
			return err
		}
	}
	for _, username := range args {
		err = server.DeleteUser(username)
		if err != nil {
			return err
		}
	}
	err = authConfigSvc.SaveConfig()
	if err != nil {
		return err
	}
	log.Infof("Deleted API tokens for users: %s for Git server %s at %s from local settings\n",
		util.ColorInfo(strings.Join(args, ", ")), util.ColorInfo(server.Name), util.ColorInfo(server.URL))
	return nil
}
