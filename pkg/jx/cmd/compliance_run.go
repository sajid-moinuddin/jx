package cmd

import (
	"io"

	"github.com/heptio/sonobuoy/pkg/client"
	"github.com/heptio/sonobuoy/pkg/config"
	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1/terminal"
	"k8s.io/api/core/v1"
)

var (
	complianceRuntLong = templates.LongDesc(`
		Runs the compliance tests
	`)

	complianceRunExample = templates.Examples(`
		# Run the compliance tests
		jx compliance run
	`)
)

// ComplianceRuntOptions options for "compliance run" command
type ComplianceRunOptions struct {
	CommonOptions
}

// NewCmdComplianceRun creates a command object for the "compliance run" action, which
// starts the E2E compliance tests
func NewCmdComplianceRun(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := &ComplianceRunOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			In:      in,
			Out:     out,
			Err:     errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "run",
		Short:   "Runs the compliance tests",
		Long:    complianceRuntLong,
		Example: complianceRunExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()

			CheckErr(err)
		},
	}

	return cmd
}

// Run implements the "compliance run" command
func (o *ComplianceRunOptions) Run() error {
	cc, err := o.Factory.CreateComplianceClient()
	if err != nil {
		return errors.Wrap(err, "could not create the compliance client")
	}
	cfg := o.config()
	if err := cc.Run(cfg); err != nil {
		return errors.Wrap(err, "failed to start the compliance tests")
	}
	return nil
}

func (o *ComplianceRunOptions) config() *client.RunConfig {
	modeName := client.Conformance
	mode := modeName.Get()
	genCfg := &client.GenConfig{
		E2EConfig:            &mode.E2EConfig,
		Config:               o.getConfigWithMode(modeName),
		Image:                complianceImage,
		Namespace:            complianceNamespace,
		EnableRBAC:           true,
		ImagePullPolicy:      string(v1.PullAlways),
		KubeConformanceImage: kubeConformanceImage,
	}
	return &client.RunConfig{
		GenConfig: *genCfg,
	}
}

func (o *ComplianceRunOptions) getConfigWithMode(mode client.Mode) *config.Config {
	cfg := config.New()
	modeConfig := mode.Get()
	if modeConfig != nil {
		cfg.PluginSelections = modeConfig.Selectors
	}
	return cfg
}
