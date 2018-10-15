package cmd

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/blang/semver"
	"github.com/jenkins-x/jx/pkg/kube"

	"github.com/jenkins-x/jx/pkg/util"

	"github.com/jenkins-x/jx/pkg/log"

	typev1 "github.com/jenkins-x/jx/pkg/client/clientset/versioned/typed/jenkins.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	jenkinsv1 "github.com/jenkins-x/jx/pkg/apis/jenkins.io/v1"

	"github.com/ghodss/yaml"

	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1/terminal"
)

//const upstreamExtensionsRepositoryUrl = "https://raw.githubusercontent.com/jenkins-x/jenkins-x-extensions/master/jenkins-x-extensions-repository.lock.yaml"
const upstreamExtensionsRepositoryUrl = "github.com/jenkins-x/jenkins-x-extensions"
const extensionsConfigDefaultConfigMap = "jenkins-x-extensions"

var (
	upgradeExtensionsLong = templates.LongDesc(`
		Upgrades the Jenkins X extensions available to this Jenkins X install if there are new versions available
`)

	upgradeExtensionsExample = templates.Examples(`
		
		# upgrade extensions
		jx upgrade extensions

	`)
)

type UpgradeExtensionsOptions struct {
	CreateOptions
	Filter                   string
	ExtensionsRepository     string
	ExtensionsRepositoryFile string
}

func NewCmdUpgradeExtensions(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := &UpgradeExtensionsOptions{
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
		Use:     "extensions",
		Short:   "Upgrades the Jenkins X extensions available to this Jenkins X install if there are new versions available",
		Long:    upgradeExtensionsLong,
		Example: upgradeBInariesExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}
	cmd.AddCommand(NewCmdUpgradeExtensionsRepository(f, in, out, errOut))
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "", false, "Enable verbose logging")
	cmd.Flags().StringVarP(&options.ExtensionsRepository, "extensions-repository", "", "", "Specify the extensions repository git repo to read from. Accepts github.com/<org>/<repo>")
	cmd.Flags().StringVarP(&options.ExtensionsRepositoryFile, "extensions-repository-file", "", "", "Specify the extensions repository yaml file to read from")
	return cmd
}

func (o *UpgradeExtensionsOptions) Run() error {

	apisClient, err := o.CreateApiExtensionsClient()
	if err != nil {
		return err
	}
	err = kube.RegisterExtensionCRD(apisClient)
	if err != nil {
		return err
	}

	extensionsRepository := jenkinsv1.ExtensionRepositoryLockList{}
	var bs []byte

	if o.ExtensionsRepositoryFile != "" {
		path := o.ExtensionsRepositoryFile
		// if it starts with a ~ it's the users homedir
		if strings.HasPrefix(path, "~") {
			usr, err := user.Current()
			if err == nil {
				path = filepath.Join(usr.HomeDir, strings.TrimPrefix(path, "~"))
			}
		}
		// Perhaps it's an absolute file path
		bs, err = ioutil.ReadFile(path)
		if err != nil {
			// Perhaps it's a relative path
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			log.Infof("Updating extensions from %s\n", path)
			bs, err = ioutil.ReadFile(filepath.Join(cwd, path))
			if err != nil {
				return errors.New(fmt.Sprintf("Unable to open Extensions Repository at %s", path))
			}
		}
	} else {
		extensionsRepository := o.ExtensionsRepository
		if extensionsRepository == "" {
			extensionsRepository = "github.com/jenkins-x/jenkins-x-extensions"
		}
		if strings.HasPrefix(extensionsRepository, "github.com") {
			_, repoInfo, err := o.createGitProviderForURLWithoutKind(extensionsRepository)
			if err != nil {
				return err
			}
			resolvedTag, err := util.GetLatestTagFromGitHub(repoInfo.Organisation, repoInfo.Name)
			if err != nil {
				return err
			}
			extensionsRepository = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/jenkins-x-extensions-repository.lock.yaml", repoInfo.Organisation, repoInfo.Name, resolvedTag)
		}
		log.Infof("Updating extensions from %s\n", extensionsRepository)
		httpClient := &http.Client{Timeout: 10 * time.Second}
		resp, err := httpClient.Get(fmt.Sprintf("%s?version=%d", extensionsRepository, time.Now().UnixNano()/int64(time.Millisecond)))

		defer resp.Body.Close()

		bs, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
	}

	err = yaml.Unmarshal(bs, &extensionsRepository)
	if err != nil {
		return err
	}
	log.Infof("Updating to Extension Repository version %s\n", util.ColorInfo(extensionsRepository.Version))
	client, ns, err := o.Factory.CreateJXClient()
	if err != nil {
		return err
	}
	extensionsClient := client.JenkinsV1().Extensions(ns)
	kubeClient, curNs, err := o.KubeClient()
	if err != nil {
		return err
	}
	extensionsConfig, err := (&jenkinsv1.ExtensionConfigList{}).LoadFromConfigMap(extensionsConfigDefaultConfigMap, kubeClient, curNs)
	if err != nil {
		return err
	}

	availableExtensionsUUIDLookup := make(map[string]jenkinsv1.ExtensionSpec, 0)
	for _, e := range extensionsRepository.Extensions {
		availableExtensionsUUIDLookup[e.UUID] = e
	}

	installedExtensions, err := o.GetInstalledExtensions(extensionsClient)
	if err != nil {
		return err
	}
	// This will cause o.devNamespace to be populated
	_, _, err = o.JXClientAndDevNamespace()
	if err != nil {
		return err
	}
	needsUpstalling := make([]jenkinsv1.ExtensionExecution, 0)
	for _, e := range extensionsRepository.Extensions {
		// TODO this is not very efficient probably
		for _, c := range extensionsConfig.Extensions {
			if c.Name == e.Name && c.Namespace == e.Namespace {
				n, err := o.UpsertExtension(e, extensionsClient, installedExtensions, c, availableExtensionsUUIDLookup, 0, 0)
				if err != nil {
					return err
				}
				needsUpstalling = append(needsUpstalling, n...)
				break
			}
		}
	}
	for _, n := range needsUpstalling {
		envVars := ""
		if len(n.EnvironmentVariables) > 0 {
			envVarsFormatted := new(bytes.Buffer)
			for _, envVar := range n.EnvironmentVariables {
				fmt.Fprintf(envVarsFormatted, "%s=%s, ", envVar.Name, envVar.Value)
			}
			envVars = fmt.Sprintf("with environment variables [ %s ]", util.ColorInfo(strings.TrimSuffix(envVarsFormatted.String(), ", ")))
		}

		log.Infof("Preparing %s %s\n", util.ColorInfo(n.FullyQualifiedName()), envVars)
		n.Execute(o.Verbose)
	}
	return nil
}

func (o *UpgradeExtensionsOptions) UpsertExtension(extension jenkinsv1.ExtensionSpec, extensions typev1.ExtensionInterface, installedExtensions map[string]jenkinsv1.Extension, extensionConfig jenkinsv1.ExtensionConfig, lookup map[string]jenkinsv1.ExtensionSpec, depth int, initialIndent int) (needsUpstalling []jenkinsv1.ExtensionExecution, err error) {
	result := make([]jenkinsv1.ExtensionExecution, 0)
	indent := ((depth - 1) * 2) + initialIndent

	// TODO Validate extension
	newVersion, err := semver.Parse(extension.Version)
	if err != nil {
		return result, err
	}
	existing, ok := installedExtensions[extension.UUID]
	if !ok {
		// Check for a name conflict
		res, err := extensions.Get(extension.FullyQualifiedKebabName(), metav1.GetOptions{})
		if err == nil {
			return result, errors.New(fmt.Sprintf("Extension %s has changed UUID. It used to have UUID %s and now has UUID %s. If this is correct, then you should manually remove the extension using\n"+
				"\n"+
				"  kubectl delete ext %s\n"+
				"\n"+
				"If this is not correct, then contact the extension maintainer and inform them of this change.", util.ColorWarning(extension.FullyQualifiedName()), util.ColorWarning(res.Spec.UUID), util.ColorWarning(extension.UUID), extension.FullyQualifiedKebabName()))
		}
		// Doesn't exist
		res, err = extensions.Create(&jenkinsv1.Extension{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf(extension.FullyQualifiedKebabName()),
			},
			Spec: extension,
		})
		if depth == 0 {
			initialIndent = 7
			log.Infof("Adding %s version %s\n", util.ColorInfo(extension.FullyQualifiedName()), util.ColorInfo(newVersion))
		} else {
			log.Infof("%s└ %s version %s\n", strings.Repeat(" ", indent), util.ColorInfo(extension.FullyQualifiedName()), util.ColorInfo(extension.Version))
		}
		if err != nil {
			return result, err
		}
		if o.Contains(extension.When, jenkinsv1.ExtensionWhenInstall) {
			e, _, err := extension.ToExecutable(extensionConfig.Parameters, o.devNamespace)
			if err != nil {
				return result, err
			}
			result = append(result, e)
		}
	}
	// TODO Handle uninstalling existing extension if name has changed but UUID hasn't
	if existing.Spec.Version != "" {
		existingVersion, err := semver.Parse(existing.Spec.Version)
		if err != nil {
			return result, err
		}
		if existingVersion.LT(newVersion) {
			existing.Spec = extension
			_, err := extensions.Update(&existing)
			if err != nil {
				return result, err
			}
			if o.Contains(extension.When, jenkinsv1.ExtensionWhenUpgrade) {
				e, _, err := extension.ToExecutable(extensionConfig.Parameters, o.devNamespace)
				if err != nil {
					return result, err
				}
				result = append(result, e)
			}
			if depth == 0 {
				initialIndent = 10
				log.Infof("Upgrading %s from %s to %s\n", util.ColorInfo(extension.FullyQualifiedName()), util.ColorInfo(existingVersion), util.ColorInfo(newVersion))
			} else {
				log.Infof("%s└ %s version %s\n", strings.Repeat(" ", indent), util.ColorInfo(extension.FullyQualifiedName()), util.ColorInfo(extension.Version))
			}
		}
	}

	for _, childRef := range extension.Children {
		if child, ok := lookup[childRef]; ok {
			e, err := o.UpsertExtension(child, extensions, installedExtensions, extensionConfig, lookup, depth+1, initialIndent)
			if err != nil {
				return result, err
			}
			result = append(result, e...)
		} else {
			errors.New(fmt.Sprintf("Unable to locate extension %s", childRef))
		}
	}
	return result, nil
}

func (o *UpgradeExtensionsOptions) Contains(whens []jenkinsv1.ExtensionWhen, when jenkinsv1.ExtensionWhen) bool {
	for _, w := range whens {
		if when == w {
			return true
		}
	}
	return false
}

func (o *UpgradeExtensionsOptions) GetInstalledExtensions(extensions typev1.ExtensionInterface) (installedExtensions map[string]jenkinsv1.Extension, err error) {
	exts, err := extensions.List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	installedExtensions = make(map[string]jenkinsv1.Extension)
	for _, ext := range exts.Items {
		if ext.Spec.UUID == "" {
			return nil, errors.New(fmt.Sprintf("Extension %s does not have a UUID", util.ColorInfo(fmt.Sprintf("%s:%s", ext.Namespace, ext.Name))))
		}
		installedExtensions[ext.Spec.UUID] = ext
	}
	return installedExtensions, nil
}
