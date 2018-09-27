package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1/terminal"
)

// StepHelmApplyOptions contains the command line flags
type StepHelmApplyOptions struct {
	StepHelmOptions

	Namespace   string
	ReleaseName string
	Wait        bool
	Force       bool
}

var (
	StepHelmApplyLong = templates.LongDesc(`
		Applies the helm chart in a given directory.

		This step is usually used to apply any GitOps promotion changes into a Staging or Production cluster.
`)

	StepHelmApplyExample = templates.Examples(`
		# apply the chart in the env folder to namespace jx-staging 
		jx step helm apply --dir env --namespace jx-staging

`)
)

func NewCmdStepHelmApply(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := StepHelmApplyOptions{
		StepHelmOptions: StepHelmOptions{
			StepOptions: StepOptions{
				CommonOptions: CommonOptions{
					Factory: f,
					In:      in,
					Out:     out,
					Err:     errOut,
				},
			},
		},
	}
	cmd := &cobra.Command{
		Use:     "apply",
		Short:   "Applies the helm chart in a given directory",
		Aliases: []string{""},
		Long:    StepHelmApplyLong,
		Example: StepHelmApplyExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}
	options.addStepHelmFlags(cmd)

	cmd.Flags().StringVarP(&options.Namespace, "namespace", "", "", "The Kubernetes namespace to apply the helm chart to")
	cmd.Flags().StringVarP(&options.ReleaseName, "name", "", "", "The name of the release")
	cmd.Flags().BoolVarP(&options.Wait, "wait", "", true, "Wait for Kubernetes readiness probe to confirm deployment")
	cmd.Flags().BoolVarP(&options.Force, "force", "f", true, "Whether to to pass '--force' to helm to help deal with upgrading if a previous promote failed")
	return cmd
}

func (o *StepHelmApplyOptions) Run() error {
	var err error
	chartName := o.Dir
	dir := o.Dir
	if dir == "" {
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	// if we're in a prow job we need to clone and change dir to find the Helm Chart.yaml
	if os.Getenv(PROW_JOB_ID) != "" {
		dir, err = o.cloneProwPullRequest(dir, o.GitProvider)
		if err != nil {
			return fmt.Errorf("failed to clone pull request: %v", err)
		}
	}

	_, err = o.helmInitDependencyBuild(dir, o.defaultReleaseCharts())
	if err != nil {
		return err
	}

	helmBinary, noTiller, helmTemplate, err := o.TeamHelmBin()
	if err != nil {
		return err
	}

	ns := o.Namespace
	if ns == "" {
		ns = os.Getenv("DEPLOY_NAMESPACE")
	}
	if ns == "" {
		return fmt.Errorf("No --namespace option specified or $DEPLOY_NAMESPACE environment variable available")
	}

	releaseName := o.ReleaseName
	if releaseName == "" {
		releaseName = ns
		if helmBinary != "helm" || noTiller || helmTemplate {
			releaseName = "jx"
		}
	}

	info := util.ColorInfo
	log.Infof("Applying helm chart at %s as release name %s to namespace %s\n", info(dir), info(releaseName), info(ns))

	o.Helm().SetCWD(dir)

	if o.Wait {
		timeout := 600
		err = o.Helm().UpgradeChart(chartName, releaseName, ns, nil, true, &timeout, o.Force, true, nil, nil)
	} else {
		err = o.Helm().UpgradeChart(chartName, releaseName, ns, nil, true, nil, o.Force, false, nil, nil)
	}
	if err != nil {
		return err
	}
	return nil
}
