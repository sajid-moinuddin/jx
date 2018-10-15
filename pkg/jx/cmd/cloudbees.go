package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1/terminal"

	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/jenkins-x/jx/pkg/kube"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/pkg/browser"
)

type CloudBeesOptions struct {
	CommonOptions

	OnlyViewURL bool
}

var (
	cdx_long = templates.LongDesc(`
		Opens the CloudBees app for Kubernetes in a browser.

		Which helps you visualise your CI/CD pipelines, apps, environments and teams.

		For more information please see [https://www.cloudbees.com/blog/want-help-build-cloudbees-kubernetes-jenkins-x](https://www.cloudbees.com/blog/want-help-build-cloudbees-kubernetes-jenkins-x)
`)
	cdx_example = templates.Examples(`
		# Open the CDX dashboard in a browser
		jx cloudbees

		# Print the Jenkins X console URL but do not open a browser
		jx console -u`)
)

func NewCmdCloudBees(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := &CloudBeesOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			In:      in,
			Out:     out,
			Err:     errOut,
		},
	}
	cmd := &cobra.Command{
		Use:     "cloudbees",
		Short:   "Opens the CloudBees app for Kubernetes for visualising CI/CD and your environments",
		Long:    cdx_long,
		Example: cdx_example,
		Aliases: []string{"cloudbee", "cb", "cdx"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}
	cmd.AddCommand(NewCmdCloudBeesPipeline(f, in, out, errOut))
	cmd.Flags().BoolVarP(&options.OnlyViewURL, "url", "u", false, "Only displays and the URL and does not open the browser")
	return cmd
}

func (o *CloudBeesOptions) Run() error {
	url, err := o.GetBaseURL()
	if err != nil {
		return err
	}
	return o.OpenURL(url, "CloudBees")
}

func (o *CloudBeesOptions) GetBaseURL() (url string, err error) {
	client, _, err := o.KubeClient()
	if err != nil {
		return "", err
	}
	url, err = kube.GetServiceURLFromName(client, kube.ServiceCloudBees, defaultCloudBeesNamespace)
	if err != nil {
		return "", fmt.Errorf("%s\n\nDid you install the CloudBees addon via: %s\n\nFor more information see: %s", err, util.ColorInfo("jx create addon cloudbees"), util.ColorInfo("https://www.cloudbees.com/blog/want-help-build-cloudbees-kubernetes-jenkins-x"))
	}

	if url == "" {
		url, err = kube.GetServiceURLFromName(client, fmt.Sprintf("sso-%s", kube.ServiceCloudBees), defaultCloudBeesNamespace)
		if err != nil {
			return "", fmt.Errorf("%s\n\nDid you install the CloudBees addon via: %s\n\nFor more information see: %s", err, util.ColorInfo("jx create addon cloudbees"), util.ColorInfo("https://www.cloudbees.com/blog/want-help-build-cloudbees-kubernetes-jenkins-x"))
		}
	}
	return url, nil
}

func (o *CloudBeesOptions) Open(name string, label string) error {
	url, err := o.findService(name)
	if err != nil {
		return err
	}
	return o.OpenURL(url, label)
}

func (o *CloudBeesOptions) OpenURL(url string, label string) error {
	// TODO Logger
	fmt.Fprintf(o.Out, "%s: %s\n", label, util.ColorInfo(url))
	if !o.OnlyViewURL {
		browser.OpenURL(url)
	}
	return nil
}
