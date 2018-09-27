package cmd

import (
	"io"
	"os/user"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1/terminal"

	"github.com/jenkins-x/jx/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/jenkins-x/jx/pkg/kube"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"k8s.io/api/apps/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetApplicationsOptions containers the CLI options
type GetApplicationsOptions struct {
	CommonOptions

	Namespace   string
	Environment string
	HideUrl     bool
	HidePod     bool
	Previews    bool
}

var (
	get_version_long = templates.LongDesc(`
		Display applications across environments.
`)

	get_version_example = templates.Examples(`
		# List applications, their URL and pod counts for all environments
		jx get apps

		# List applications only in the Staging environment
		jx get apps -e staging

		# List applications only in the Production environment
		jx get apps -e production

		# List applications only in a specific namespace
		jx get apps -n jx-staging

		# List applications hiding the URLs
		jx get apps -u

		# List applications just showing the versions (hiding urls and pod counts)
		jx get apps -u -p
	`)
)

// NewCmdGetApplications creates the new command for: jx get version
func NewCmdGetApplications(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := &GetApplicationsOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			In:      in,
			Out:     out,
			Err:     errOut,
		},
	}
	cmd := &cobra.Command{
		Use:     "applications",
		Short:   "Display one or many Applications and their versions",
		Aliases: []string{"app", "apps", "version", "versions"},
		Long:    get_version_long,
		Example: get_version_example,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}
	cmd.Flags().BoolVarP(&options.HideUrl, "url", "u", false, "Hide the URLs")
	cmd.Flags().BoolVarP(&options.HidePod, "pod", "p", false, "Hide the pod counts")
	cmd.Flags().BoolVarP(&options.Previews, "preview", "w", false, "Show preview environments only")
	cmd.Flags().StringVarP(&options.Environment, "env", "e", "", "Filter applications in the given environment")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", "", "Filter applications in the given namespace")
	return cmd
}

type EnvApps struct {
	Environment v1.Environment
	Apps        map[string]v1beta1.Deployment
}

// Run implements this command
func (o *GetApplicationsOptions) Run() error {
	f := o.Factory
	client, currentNs, err := f.CreateJXClient()
	if err != nil {
		return err
	}
	kubeClient, _, err := o.KubeClient()
	if err != nil {
		return err
	}
	u, err := user.Current()
	if err != nil {
		return err
	}
	ns, _, err := kube.GetDevNamespace(kubeClient, currentNs)
	if err != nil {
		return err
	}
	envList, err := client.JenkinsV1().Environments(ns).List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	kube.SortEnvironments(envList.Items)

	namespaces := []string{}
	envApps := []EnvApps{}
	envNames := []string{}
	apps := []string{}
	for _, env := range envList.Items {
		isPreview := env.Spec.Kind == v1.EnvironmentKindTypePreview
		shouldShow := isPreview
		if !o.Previews {
			shouldShow = !shouldShow
		}
		if shouldShow &&
			(o.Environment == "" || o.Environment == env.Name) &&
			(o.Namespace == "" || o.Namespace == env.Spec.Namespace) {
			ens := env.Spec.Namespace
			namespaces = append(namespaces, ens)
			if ens != "" && env.Name != kube.LabelValueDevEnvironment {
				envNames = append(envNames, env.Name)
				m, err := kube.GetDeployments(kubeClient, ens)
				if err == nil {
					envApp := EnvApps{
						Environment: env,
						Apps:        map[string]v1beta1.Deployment{},
					}
					envApps = append(envApps, envApp)
					for k, d := range m {
						appName := kube.GetAppName(k, ens)
						if env.Spec.Kind == v1.EnvironmentKindTypeEdit {
							if appName == kube.DeploymentExposecontrollerService || env.Spec.PreviewGitSpec.User.Username != u.Username {
								continue
							}
							appName = kube.GetEditAppName(appName)
						} else if env.Spec.Kind == v1.EnvironmentKindTypePreview {
							appName = env.Spec.PullRequestURL
						}
						envApp.Apps[appName] = d
						if util.StringArrayIndex(apps, appName) < 0 {
							apps = append(apps, appName)
						}
					}
				}
			}
		}
	}
	util.ReverseStrings(namespaces)
	if len(apps) == 0 {
		log.Infof("No applications found in environments %s\n", strings.Join(envNames, ", "))
		return nil
	}
	sort.Strings(apps)

	table := o.CreateTable()
	title := "APPLICATION"
	if o.Previews {
		title = "PULL REQUESTS"
	}
	titles := []string{title}
	for _, ea := range envApps {
		envName := ea.Environment.Name
		if ea.Environment.Spec.Kind == v1.EnvironmentKindTypeEdit {
			envName = "Edit"
		}
		if ea.Environment.Spec.Kind != v1.EnvironmentKindTypePreview {
			titles = append(titles, strings.ToUpper(envName))
		}
		if !o.HidePod {
			titles = append(titles, "PODS")
		}
		if !o.HideUrl {
			titles = append(titles, "URL")
		}
	}
	table.AddRow(titles...)

	for _, appName := range apps {
		row := []string{appName}
		for _, ea := range envApps {
			version := ""
			d := ea.Apps[appName]
			version = kube.GetVersion(&d.ObjectMeta)
			if ea.Environment.Spec.Kind != v1.EnvironmentKindTypePreview {
				row = append(row, version)
			}
			if !o.HidePod {
				pods := ""
				replicas := ""
				ready := d.Status.ReadyReplicas
				if d.Spec.Replicas != nil && ready > 0 {
					replicas = formatInt32(*d.Spec.Replicas)
					pods = formatInt32(ready) + "/" + replicas
				}
				row = append(row, pods)
			}
			if !o.HideUrl {
				url, _ := kube.FindServiceURL(kubeClient, d.Namespace, appName)
				if url == "" {
					url, _ = kube.FindServiceURL(kubeClient, d.Namespace, d.Name)
				}
				if url == "" {
					// handle helm3
					chart := d.Labels["chart"]
					if chart != "" {
						idx := strings.LastIndex(chart, "-")
						if idx > 0 {
							svcName := chart[0:idx]
							if svcName != appName && svcName != d.Name {
								url, _ = kube.FindServiceURL(kubeClient, d.Namespace, svcName)
							}
						}
					}
				}
				row = append(row, url)
			}
		}
		table.AddRow(row...)
	}
	table.Render()
	return nil
}
