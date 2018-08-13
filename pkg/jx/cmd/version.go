package cmd

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/jenkins-x/jx/pkg/version"
	"github.com/spf13/cobra"
)

const (
	jxChartPrefix = "jenkins-x-platform-"
)

type VersionOptions struct {
	CommonOptions

	Container      string
	Namespace      string
	HelmTLS        bool
	NoVersionCheck bool
}

func NewCmdVersion(f Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &VersionOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			Out:     out,
			Err:     errOut,
		},
	}

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}
	/*
		cmd.Flags().BoolP("client", "c", false, "Client version only (no server required).")
		cmd.Flags().BoolP("short", "", false, "Print just the version number.")
	*/
	options.addCommonFlags(cmd)

	cmd.Flags().MarkShorthandDeprecated("client", "please use --client instead.")
	cmd.Flags().BoolVarP(&options.HelmTLS, "helm-tls", "", false, "Whether to use TLS with helm")
	cmd.Flags().BoolVarP(&options.NoVersionCheck, "no-version-check", "n", false, "Disable checking of version upgrade checks")
	return cmd
}

func (o *VersionOptions) Run() error {
	info := util.ColorInfo
	table := o.CreateTable()
	table.AddRow("NAME", "VERSION")
	table.AddRow("jx", info(version.GetVersion()))

	// Jenkins X version
	output, err := o.Helm().ListCharts()
	if err != nil {
		log.Warnf("Failed to find helm installs: %s\n", err)
	} else {
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) > 4 && strings.TrimSpace(fields[0]) == "jenkins-x" {
				for _, f := range fields[4:] {
					f = strings.TrimSpace(f)
					if strings.HasPrefix(f, jxChartPrefix) {
						chart := strings.TrimPrefix(f, jxChartPrefix)
						table.AddRow("jenkins x platform", info(chart))
					}
				}
			}
		}
	}

	// kubernetes version
	client, _, err := o.KubeClient()
	if err != nil {
		log.Warnf("Failed to connect to kubernetes: %s\n", err)
	} else {
		serverVersion, err := client.Discovery().ServerVersion()
		if err != nil {
			log.Warnf("Failed to get kubernetes server version: %s\n", err)
		} else if serverVersion != nil {
			table.AddRow("kubernetes cluster", info(serverVersion.String()))
		}
	}

	// kubectl version
	output, err = o.getCommandOutput("", "kubectl", "version", "--short")
	if err != nil {
		log.Warnf("Failed to get kubectl version: %s\n", err)
	} else {
		for i, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				v := fields[2]
				if v != "" {
					switch i {
					case 0:
						table.AddRow("kubectl", info(v))
					case 1:
						// Ignore K8S server details as we have these above
					}
				}
			}
		}
	}

	// helm version
	output, err = o.Helm().Version(o.HelmTLS)
	if err != nil {
		log.Warnf("Failed to get helm version: %s\n", err)
	} else {
		helmBinary := o.Helm().HelmBinary()
		if helmBinary == "helm3" {
			table.AddRow("helm client", info(output))
		} else {
			for i, line := range strings.Split(output, "\n") {
				fields := strings.Fields(line)
				if len(fields) > 1 {
					v := fields[1]
					if v != "" {
						switch i {
						case 0:
							table.AddRow("helm client", info(v))
						case 1:
							table.AddRow("helm server", info(v))
						}
					}
				}
			}
		}
	}

	// git version
	version, err := o.Git().Version()
	if err != nil {
		log.Warnf("Failed to get git version: %s\n", err)
	} else {
		table.AddRow("git", info(version))
	}

	table.Render()

	if !o.NoVersionCheck {
		return o.VersionCheck()
	}
	return nil
}

func (o *VersionOptions) VersionCheck() error {
	newVersion, err := o.GetLatestJXVersion()
	if err != nil {
		return err
	}

	currentVersion, err := version.GetSemverVersion()
	if err != nil {
		return err
	}

	if newVersion.GT(currentVersion) {
		// Do not ask to update if we are using a dev build...
		for _, x := range currentVersion.Pre {
			if x.VersionStr == "dev" {
				return nil
			}
		}
		app := util.ColorInfo("jx")
		log.Warnf("\nA new %s version is available: %s\n", app, util.ColorInfo(newVersion.String()))

		if o.BatchMode {
			log.Warnf("To upgrade to this new version use: %s\n", util.ColorInfo("jx upgrade cli"))
		} else {
			message := fmt.Sprintf("Would you like to upgrade to the new %s version?", app)
			if util.Confirm(message, true, "Please indicate if you would like to upgrade the binary version.") {
				return o.UpgradeCli()
			}
		}
	}
	return nil
}

func (o *VersionOptions) UpgradeCli() error {
	options := &UpgradeCLIOptions{
		CreateOptions: CreateOptions{
			CommonOptions: o.CommonOptions,
		},
	}
	return options.Run()
}

func extractSemVer(text string) string {
	re, err := regexp.Compile(".*SemVer:\"(.*)\"")
	if err != nil {
		return ""
	}
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
