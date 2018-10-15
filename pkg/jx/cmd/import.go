package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/cenkalti/backoff"
	"github.com/jenkins-x/jx/pkg/cloud/amazon"
	"github.com/pkg/errors"

	"github.com/Azure/draft/pkg/draft/draftpath"
	"github.com/jenkins-x/draft-repo/pkg/draft/pack"
	"github.com/jenkins-x/golang-jenkins"
	"github.com/jenkins-x/jx/pkg/auth"
	"github.com/jenkins-x/jx/pkg/config"
	jxdraft "github.com/jenkins-x/jx/pkg/draft"
	"github.com/jenkins-x/jx/pkg/gits"
	"github.com/jenkins-x/jx/pkg/jenkins"
	"github.com/jenkins-x/jx/pkg/jx/cmd/templates"
	"github.com/jenkins-x/jx/pkg/kube"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/spf13/cobra"
	"gopkg.in/AlecAivazis/survey.v1"
	"gopkg.in/AlecAivazis/survey.v1/terminal"
	gitcfg "gopkg.in/src-d/go-git.v4/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//_ "github.com/Azure/draft/pkg/linguist"
	"time"

	"github.com/denormal/go-gitignore"
	"github.com/jenkins-x/jx/pkg/prow"
	"gopkg.in/yaml.v2"
)

const (
	// PlaceHolderAppName placeholder for app name
	PlaceHolderAppName = "REPLACE_ME_APP_NAME"
	// PlaceHolderGitProvider placeholder for git provider
	PlaceHolderGitProvider = "REPLACE_ME_GIT_PROVIDER"
	// PlaceHolderOrg placeholder for org
	PlaceHolderOrg = "REPLACE_ME_ORG"
	// PlaceHolderDockerRegistryOrg placeholder for docker registry
	PlaceHolderDockerRegistryOrg = "REPLACE_ME_DOCKER_REGISTRY_ORG"

	// JenkinsfileBackupSuffix the suffix used by Jenkins for backups
	JenkinsfileBackupSuffix = ".backup"

	minimumMavenDeployVersion = "2.8.2"

	defaultGitIgnoreFile = `
.project
.classpath
.idea
.cache
.DS_Store
*.im?
target
work
`
)

// CallbackFn callback function
type CallbackFn func() error

// ImportOptions options struct for jx import
type ImportOptions struct {
	CommonOptions

	RepoURL string

	Dir                     string
	Organisation            string
	Repository              string
	Credentials             string
	AppName                 string
	GitHub                  bool
	DryRun                  bool
	SelectAll               bool
	DisableDraft            bool
	DisableJenkinsfileCheck bool
	SelectFilter            string
	Jenkinsfile             string
	BranchPattern           string
	GitRepositoryOptions    gits.GitRepositoryOptions
	ImportGitCommitMessage  string
	ListDraftPacks          bool
	DraftPack               string
	DockerRegistryOrg       string

	DisableDotGitSearch   bool
	InitialisedGit        bool
	Jenkins               gojenkins.JenkinsClient
	GitConfDir            string
	GitServer             *auth.AuthServer
	GitUserAuth           *auth.UserAuth
	GitProvider           gits.GitProvider
	PostDraftPackCallback CallbackFn
	DisableMaven          bool
	PipelineUserName      string
	PipelineServer        string
}

var (
	importLong = templates.LongDesc(`
		Imports a local folder or Git repository into Jenkins X.

		If you specify no other options or arguments then the current directory is imported.
	    Or you can use '--dir' to specify a directory to import.

	    You can specify the git URL as an argument.
	    
		For more documentation see: [https://jenkins-x.io/developing/import/](https://jenkins-x.io/developing/import/)
	    
	`)

	importExample = templates.Examples(`
		# Import the current folder
		jx import

		# Import a different folder
		jx import /foo/bar

		# Import a Git repository from a URL
		jx import --url https://github.com/jenkins-x/spring-boot-web-example.git

        # Select a number of repositories from a GitHub organisation
		jx import --github --org myname 

        # Import all repositories from a GitHub organisation selecting ones to not import
		jx import --github --org myname --all 

        # Import all repositories from a GitHub organisation which contain the text foo
		jx import --github --org myname --all --filter foo 
		`)
)

// NewCmdImport the cobra command for jx import
func NewCmdImport(f Factory, in terminal.FileReader, out terminal.FileWriter, errOut io.Writer) *cobra.Command {
	options := &ImportOptions{
		CommonOptions: CommonOptions{
			Factory: f,
			In:      in,
			Out:     out,
			Err:     errOut,
		},
	}
	cmd := &cobra.Command{
		Use:     "import",
		Short:   "Imports a local project or Git repository into Jenkins",
		Long:    importLong,
		Example: importExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&options.RepoURL, "url", "u", "", "The git clone URL to clone into the current directory and then import")
	cmd.Flags().BoolVarP(&options.GitHub, "github", "", false, "If you wish to pick the repositories from GitHub to import")
	cmd.Flags().BoolVarP(&options.SelectAll, "all", "", false, "If selecting projects to import from a Git provider this defaults to selecting them all")
	cmd.Flags().StringVarP(&options.SelectFilter, "filter", "", "", "If selecting projects to import from a Git provider this filters the list of repositories")

	options.addImportFlags(cmd, false)

	return cmd
}

func (options *ImportOptions) addImportFlags(cmd *cobra.Command, createProject bool) {
	notCreateProject := func(text string) string {
		if createProject {
			return ""
		}
		return text
	}
	cmd.Flags().StringVarP(&options.Organisation, "org", "", "", "Specify the Git provider organisation to import the project into (if it is not already in one)")
	cmd.Flags().StringVarP(&options.Repository, "name", "", notCreateProject("n"), "Specify the Git repository name to import the project into (if it is not already in one)")
	cmd.Flags().StringVarP(&options.Credentials, "credentials", notCreateProject("c"), "", "The Jenkins credentials name used by the job")
	cmd.Flags().StringVarP(&options.Jenkinsfile, "jenkinsfile", notCreateProject("j"), "", "The name of the Jenkinsfile to use. If not specified then 'Jenkinsfile' will be used")
	cmd.Flags().BoolVarP(&options.DryRun, "dry-run", "", false, "Performs local changes to the repo but skips the import into Jenkins X")
	cmd.Flags().BoolVarP(&options.DisableDraft, "no-draft", "", false, "Disable Draft from trying to default a Dockerfile and Helm Chart")
	cmd.Flags().BoolVarP(&options.DisableJenkinsfileCheck, "no-jenkinsfile", "", false, "Disable defaulting a Jenkinsfile if its missing")
	cmd.Flags().StringVarP(&options.ImportGitCommitMessage, "import-commit-message", "", "", "Should we override the Jenkinsfile in the project?")
	cmd.Flags().StringVarP(&options.BranchPattern, "branches", "", "", "The branch pattern for branches to trigger CI/CD pipelines on")
	cmd.Flags().BoolVarP(&options.ListDraftPacks, "list-packs", "", false, "list available draft packs")
	cmd.Flags().StringVarP(&options.DraftPack, "pack", "", "", "The name of the pack to use")
	cmd.Flags().StringVarP(&options.DockerRegistryOrg, "docker-registry-org", "", "", "The name of the docker registry organisation to use. If not specified then the Git provider organisation will be used")
	cmd.Flags().StringVarP(&options.ExternalJenkinsBaseURL, "external-jenkins-url", "", "", "The jenkins url that an external git provider needs to use")

	options.addCommonFlags(cmd)
	addGitRepoOptionsArguments(cmd, &options.GitRepositoryOptions)
}

// Run executes the command
func (options *ImportOptions) Run() error {
	if options.ListDraftPacks {
		packs, err := options.allDraftPacks()
		if err != nil {
			log.Error(err.Error())
			return err
		}
		log.Infoln("Available draft packs:")
		for i := 0; i < len(packs); i++ {
			log.Infof(packs[i] + "\n")
		}
		return nil
	}

	options.Factory.SetBatch(options.BatchMode)

	var err error
	isProw := false
	if !options.DryRun {
		_, _, err = options.KubeClient()
		if err != nil {
			return err
		}

		_, _, err = options.JXClient()
		if err != nil {
			return err
		}

		apisClient, err := options.CreateApiExtensionsClient()
		if err != nil {
			return err
		}
		err = kube.RegisterEnvironmentCRD(apisClient)
		if err != nil {
			return err
		}

		isProw, err = options.isProw()
		if err != nil {
			return err
		}

		if !isProw {
			options.Jenkins, err = options.JenkinsClient()
			if err != nil {
				return err
			}
		}
	}
	err = options.DefaultsFromTeamSettings()
	if err != nil {
		return err
	}

	var userAuth *auth.UserAuth
	if options.GitProvider == nil {
		authConfigSvc, err := options.CreateGitAuthConfigServiceDryRun(options.DryRun)
		if err != nil {
			return err
		}
		config := authConfigSvc.Config()
		var server *auth.AuthServer
		if options.RepoURL != "" {
			gitInfo, err := gits.ParseGitURL(options.RepoURL)
			if err != nil {
				return err
			}
			serverURL := gitInfo.HostURLWithoutUser()
			server = config.GetOrCreateServer(serverURL)
		} else {
			server, err = config.PickOrCreateServer(gits.GitHubURL, options.GitRepositoryOptions.ServerURL, "Which Git service do you wish to use", options.BatchMode, options.In, options.Out, options.Err)
			if err != nil {
				return err
			}
		}
		// Get the org in case there is more than one user auth on the server and batchMode is true
		org := options.getOrganisationOrCurrentUser()
		userAuth, err = config.PickServerUserAuth(server, "Git user name:", options.BatchMode, org, options.In, options.Out, options.Err)
		if err != nil {
			return err
		}
		if server.Kind == "" {
			server.Kind, err = options.GitServerHostURLKind(server.URL)
			if err != nil {
				return err
			}
		}
		if userAuth.IsInvalid() {
			f := func(username string) error {
				options.Git().PrintCreateRepositoryGenerateAccessToken(server, username, options.Out)
				return nil
			}
			err = config.EditUserAuth(server.Label(), userAuth, userAuth.Username, true, options.BatchMode, f, options.In, options.Out, options.Err)
			if err != nil {
				return err
			}

			// TODO lets verify the auth works?
			if userAuth.IsInvalid() {
				return fmt.Errorf("Authentication has failed for user %v. Please check the user's access credentials and try again", userAuth.Username)
			}
		}
		err = authConfigSvc.SaveUserAuth(server.URL, userAuth)
		if err != nil {
			return fmt.Errorf("Failed to store git auth configuration %s", err)
		}

		options.GitServer = server
		options.GitUserAuth = userAuth
		options.GitProvider, err = gits.CreateProvider(server, userAuth, options.Git())
		if err != nil {
			return err
		}
	}
	if options.GitHub {
		return options.ImportProjectsFromGitHub()
	}
	if options.Dir == "" {
		args := options.Args
		if len(args) > 0 {
			options.Dir = args[0]
		} else {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			options.Dir = dir
		}
	}

	checkForJenkinsfile := options.Jenkinsfile == "" && !options.DisableJenkinsfileCheck
	shouldClone := checkForJenkinsfile || !options.DisableDraft

	if options.RepoURL != "" {
		if shouldClone {
			// lets make sure there's a .git at the end for GitHub URLs
			err = options.CloneRepository()
			if err != nil {
				return err
			}
		}
	} else {
		err = options.DiscoverGit()
		if err != nil {
			return err
		}

		if options.RepoURL == "" {
			err = options.DiscoverRemoteGitURL()
			if err != nil {
				return err
			}
		}
	}

	if options.AppName == "" {
		if options.RepoURL != "" {
			info, err := gits.ParseGitURL(options.RepoURL)
			if err != nil {
				log.Warnf("Failed to parse git URL %s : %s\n", options.RepoURL, err)
			} else {
				options.AppName = info.Name
			}
		}
	}
	if options.AppName == "" {
		dir, err := filepath.Abs(options.Dir)
		if err != nil {
			return err
		}
		_, options.AppName = filepath.Split(dir)
	}
	options.AppName = kube.ToValidName(strings.ToLower(options.AppName))

	if !options.DisableDraft {
		err = options.DraftCreate()
		if err != nil {
			return err
		}

	}
	err = options.fixDockerIgnoreFile()
	if err != nil {
		return err
	}

	err = options.fixMaven()
	if err != nil {
		return err
	}

	if options.RepoURL == "" {
		if !options.DryRun {
			err = options.CreateNewRemoteRepository()
			if err != nil {
				return err
			}
		}
	} else {
		if shouldClone {
			err = options.Git().Push(options.Dir)
			if err != nil {
				return err
			}
		}
	}

	if options.DryRun {
		log.Infoln("dry-run so skipping import to Jenkins X")
		return nil
	}

	if !isProw {
		err = options.checkChartmuseumCredentialExists()
		if err != nil {
			return err
		}
	}

	return options.doImport()
}

// ImportProjectsFromGitHub import projects from github
func (options *ImportOptions) ImportProjectsFromGitHub() error {
	repos, err := gits.PickRepositories(options.GitProvider, options.Organisation, "Which repositories do you want to import", options.SelectAll, options.SelectFilter, options.In, options.Out, options.Err)
	if err != nil {
		return err
	}

	log.Infoln("Selected repositories")
	for _, r := range repos {
		o2 := ImportOptions{
			CommonOptions:           options.CommonOptions,
			Dir:                     options.Dir,
			RepoURL:                 r.CloneURL,
			Organisation:            options.Organisation,
			Repository:              r.Name,
			Jenkins:                 options.Jenkins,
			GitProvider:             options.GitProvider,
			DisableJenkinsfileCheck: options.DisableJenkinsfileCheck,
			DisableDraft:            options.DisableDraft,
		}
		log.Infof("Importing repository %s\n", util.ColorInfo(r.Name))
		err = o2.Run()
		if err != nil {
			return err
		}
	}
	return nil
}

// DraftCreate creates a draft
func (options *ImportOptions) DraftCreate() error {
	draftDir, err := util.DraftDir()
	if err != nil {
		return err
	}
	draftHome := draftpath.Home(draftDir)

	// lets make sure we have the latest draft packs
	initOpts := InitOptions{
		CommonOptions: options.CommonOptions,
	}
	packsDir, err := initOpts.initBuildPacks()
	if err != nil {
		return err
	}

	// TODO this is a workaround of this draft issue:
	// https://github.com/Azure/draft/issues/476
	dir := options.Dir

	defaultJenkinsfile := filepath.Join(dir, jenkins.DefaultJenkinsfile)
	jenkinsfile := defaultJenkinsfile
	withRename := false
	if options.Jenkinsfile != "" {
		jenkinsfile = filepath.Join(dir, options.Jenkinsfile)
		withRename = true
	}
	pomName := filepath.Join(dir, "pom.xml")
	gradleName := filepath.Join(dir, "build.gradle")
	jenkinsPluginsName := filepath.Join(dir, "plugins.txt")
	packagerConfigName := filepath.Join(dir, "packager-config.yml")
	lpack := ""
	customDraftPack := options.DraftPack
	if len(customDraftPack) == 0 {
		projectConfig, _, err := config.LoadProjectConfig(dir)
		if err != nil {
			return err
		}
		customDraftPack = projectConfig.BuildPack
	}

	if len(customDraftPack) > 0 {
		log.Info("trying to use draft pack: " + customDraftPack + "\n")
		lpack = filepath.Join(packsDir, customDraftPack)
		f, err := util.FileExists(lpack)
		if err != nil {
			log.Error(err.Error())
			return err
		}
		if f == false {
			log.Error("Could not find pack: " + customDraftPack + " going to try detect which pack to use")
			lpack = ""
		}

	}

	if len(lpack) == 0 {
		if exists, err := util.FileExists(pomName); err == nil && exists {
			pack, err := util.PomFlavour(pomName)
			if err != nil {
				return err
			}
			if len(pack) > 0 {
				if pack == util.LIBERTY {
					lpack = filepath.Join(packsDir, "liberty")
				} else if pack == util.APPSERVER {
					lpack = filepath.Join(packsDir, "appserver")
				} else if pack == util.DROPWIZARD {
					lpack = filepath.Join(packsDir, "dropwizard")
				} else {
					log.Warn("Do not know how to handle pack: " + pack)
				}
			} else {
				lpack = filepath.Join(packsDir, "maven")
			}

			exists, _ = util.FileExists(lpack)
			if !exists {
				log.Warn("defaulting to maven pack")
				lpack = filepath.Join(packsDir, "maven")
			}
		} else if exists, err := util.FileExists(gradleName); err == nil && exists {
			lpack = filepath.Join(packsDir, "gradle")
		} else if exists, err := util.FileExists(jenkinsPluginsName); err == nil && exists {
			lpack = filepath.Join(packsDir, "jenkins")
		} else if exists, err := util.FileExists(packagerConfigName); err == nil && exists {
			lpack = filepath.Join(packsDir, "cwp")
		} else {
			// pack detection time
			lpack, err = jxdraft.DoPackDetection(draftHome, options.Out, dir)

			if err != nil {
				return err
			}
		}
	}
	log.Success("selected pack: " + lpack + "\n")
	options.DraftPack = filepath.Base(lpack)

	chartsDir := filepath.Join(dir, "charts")
	jenkinsfileExists, err := util.FileExists(jenkinsfile)
	exists, err := util.FileExists(chartsDir)
	if exists && err == nil {
		exists, err = util.FileExists(filepath.Join(dir, "Dockerfile"))
		if exists && err == nil {
			if jenkinsfileExists || options.DisableJenkinsfileCheck {
				log.Warn("existing Dockerfile, Jenkinsfile and charts folder found so skipping 'draft create' step\n")
				return nil
			}
		}
	}
	jenkinsfileBackup := ""
	if jenkinsfileExists && options.InitialisedGit && !options.DisableJenkinsfileCheck {
		// lets copy the old Jenkinsfile in case we overwrite it
		jenkinsfileBackup = jenkinsfile + JenkinsfileBackupSuffix
		err = util.RenameFile(jenkinsfile, jenkinsfileBackup)
		if err != nil {
			return fmt.Errorf("Failed to rename old Jenkinsfile: %s", err)
		}
	} else if withRename {
		defaultJenkinsfileExists, err := util.FileExists(defaultJenkinsfile)
		if defaultJenkinsfileExists && options.InitialisedGit && !options.DisableJenkinsfileCheck {
			jenkinsfileBackup = defaultJenkinsfile + JenkinsfileBackupSuffix
			err = util.RenameFile(defaultJenkinsfile, jenkinsfileBackup)
			if err != nil {
				return fmt.Errorf("Failed to rename old Jenkinsfile: %s", err)
			}

		}
	}

	err = pack.CreateFrom(dir, lpack)
	if err != nil {
		// lets ignore draft errors as sometimes it can't find a pack - e.g. for environments
		log.Warnf("Failed to run draft create in %s due to %s", dir, err)
	}

	unpackedDefaultJenkinsfile := defaultJenkinsfile
	if unpackedDefaultJenkinsfile != jenkinsfile {
		unpackedDefaultJenkinsfileExists := false
		unpackedDefaultJenkinsfileExists, err = util.FileExists(unpackedDefaultJenkinsfile)
		if unpackedDefaultJenkinsfileExists {
			err = util.RenameFile(unpackedDefaultJenkinsfile, jenkinsfile)
			if err != nil {
				return fmt.Errorf("Failed to rename Jenkinsfile file from '%s' to '%s': %s", unpackedDefaultJenkinsfile, jenkinsfile, err)
			}
			if jenkinsfileBackup != "" {
				err = util.RenameFile(jenkinsfileBackup, defaultJenkinsfile)
				if err != nil {
					return fmt.Errorf("Failed to rename Jenkinsfile backup file: %s", err)
				}
			}
		}
	} else if jenkinsfileBackup != "" {
		// if there's no Jenkinsfile created then rename it back again!
		jenkinsfileExists, err = util.FileExists(jenkinsfile)
		if err != nil {
			log.Warnf("Failed to check for Jenkinsfile %s", err)
		} else {
			if jenkinsfileExists {
				if !options.InitialisedGit {
					err = os.Remove(jenkinsfileBackup)
					if err != nil {
						log.Warnf("Failed to remove Jenkinsfile backup %s", err)
					}
				}
			} else {
				// lets put the old one back again
				err = util.RenameFile(jenkinsfileBackup, jenkinsfile)
				if err != nil {
					return fmt.Errorf("Failed to rename Jenkinsfile backup file: %s", err)
				}
			}
		}
	}

	// lets rename the chart to be the same as our app name
	err = options.renameChartToMatchAppName()
	if err != nil {
		return err
	}

	if options.PostDraftPackCallback != nil {
		err = options.PostDraftPackCallback()
		if err != nil {
			return err
		}
	}

	gitServerName, err := gits.GetHost(options.GitProvider)
	if err != nil {
		return err
	}

	org := options.getOrganisationOrCurrentUser()
	dockerRegistryOrg := options.getDockerRegistryOrg()
	err = options.ReplacePlaceholders(gitServerName, org, dockerRegistryOrg)
	if err != nil {
		return err
	}

	// Create prow owners file
	err = options.CreateProwOwnersFile()
	if err != nil {
		return err
	}
	err = options.CreateProwOwnersAliasesFile()
	if err != nil {
		return err
	}

	err = options.Git().Add(dir, "*")
	if err != nil {
		return err
	}
	err = options.Git().CommitIfChanges(dir, "Draft create")
	if err != nil {
		return err
	}
	return nil
}

func (options *ImportOptions) getDockerRegistryOrg() string {
	dockerRegistryOrg := options.DockerRegistryOrg
	if dockerRegistryOrg == "" {
		dockerRegistryOrg = options.getOrganisationOrCurrentUser()
	}
	return dockerRegistryOrg
}

func (options *ImportOptions) getOrganisationOrCurrentUser() string {
	org := options.getOrganisation()
	if org == "" {
		org = options.getCurrentUser()
	}
	return org
}

func (options *ImportOptions) getCurrentUser() string {
	//walk through every file in the given dir and update the placeholders
	var currentUser string
	if options.GitServer != nil {
		currentUser = options.GitServer.CurrentUser
		if currentUser == "" {
			if options.GitProvider != nil {
				currentUser = options.GitProvider.CurrentUsername()
			}
		}
	}
	if currentUser == "" {
		log.Warn("No username defined for the current Git server!")
		currentUser = options.GitRepositoryOptions.Username
	}
	return currentUser
}

func (options *ImportOptions) getOrganisation() string {
	org := ""
	gitInfo, err := gits.ParseGitURL(options.RepoURL)
	if err == nil && gitInfo.Organisation != "" {
		org = gitInfo.Organisation
	} else {
		org = options.Organisation
	}
	return org
}

// CreateNewRemoteRepository creates a new remote repository
func (options *ImportOptions) CreateNewRemoteRepository() error {
	authConfigSvc, err := options.CreateGitAuthConfigService()
	if err != nil {
		return err
	}

	dir := options.Dir
	_, defaultRepoName := filepath.Split(dir)

	options.GitRepositoryOptions.Owner = options.getOrganisation()

	details, err := gits.PickNewGitRepository(options.BatchMode, authConfigSvc, defaultRepoName, &options.GitRepositoryOptions,
		options.GitServer, options.GitUserAuth, options.Git(), options.In, options.Out, options.Err)
	if err != nil {
		return err
	}
	repo, err := details.CreateRepository()
	if err != nil {
		return err
	}
	options.GitProvider = details.GitProvider

	options.RepoURL = repo.CloneURL
	pushGitURL, err := options.Git().CreatePushURL(repo.CloneURL, details.User)
	if err != nil {
		return err
	}
	err = options.Git().AddRemote(dir, "origin", pushGitURL)
	if err != nil {
		return err
	}
	err = options.Git().PushMaster(dir)
	if err != nil {
		return err
	}
	log.Infof("Pushed Git repository to %s\n\n", util.ColorInfo(repo.HTMLURL))

	// If the user creating the repo is not the pipeline user, add the pipeline user as a contributor to the repo
	if options.PipelineUserName != options.GitUserAuth.Username && options.GitServer.URL == options.PipelineServer {
		// Make the invitation
		err := options.GitProvider.AddCollaborator(options.PipelineUserName, details.Organisation, details.RepoName)
		if err != nil {
			return err
		}

		// If repo is put in an organisation that the pipeline user is not part of an invitation needs to be accepted.
		// Create a new provider for the pipeline user
		authConfig := authConfigSvc.Config()
		if err != nil {
			return err
		}
		pipelineUserAuth := authConfig.FindUserAuth(options.GitServer.URL, options.PipelineUserName)
		if pipelineUserAuth == nil {
			log.Warnf("Pipeline Git user credentials not found. %s will need to accept the invitation to collaborate\n"+
				"on %s if %s is not part of %s.\n\n",
				options.PipelineUserName, details.RepoName, options.PipelineUserName, details.Organisation)
		} else {
			pipelineServerAuth := authConfig.GetServer(authConfig.CurrentServer)
			pipelineUserProvider, err := gits.CreateProvider(pipelineServerAuth, pipelineUserAuth, options.Git())
			if err != nil {
				return err
			}

			// Get all invitations for the pipeline user
			// Wrapped in retry to not immediately fail the quickstart creation if APIs are flaky.
			f := func() error {
				invites, _, err := pipelineUserProvider.ListInvitations()
				if err != nil {
					return err
				}
				for _, x := range invites {
					// Accept all invitations for the pipeline user
					_, err = pipelineUserProvider.AcceptInvitation(*x.ID)
					if err != nil {
						return err
					}
				}
				return nil
			}
			exponentialBackOff := backoff.NewExponentialBackOff()
			timeout := 20 * time.Second
			exponentialBackOff.MaxElapsedTime = timeout
			exponentialBackOff.Reset()
			err = backoff.Retry(f, exponentialBackOff)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

// CloneRepository clones a repository
func (options *ImportOptions) CloneRepository() error {
	url := options.RepoURL
	if url == "" {
		return fmt.Errorf("no Git repository URL defined")
	}
	gitInfo, err := gits.ParseGitURL(url)
	if err != nil {
		return fmt.Errorf("failed to parse Git URL %s due to: %s", url, err)
	}
	if gitInfo.Host == gits.GitHubHost && strings.HasPrefix(gitInfo.Scheme, "http") {
		if !strings.HasSuffix(url, ".git") {
			url += ".git"
		}
		options.RepoURL = url
	}
	cloneDir, err := util.CreateUniqueDirectory(options.Dir, gitInfo.Name, util.MaximumNewDirectoryAttempts)
	if err != nil {
		return errors.Wrapf(err, "failed to create unique directory for '%s'", options.Dir)
	}
	err = options.Git().Clone(url, cloneDir)
	if err != nil {
		return errors.Wrapf(err, "failed to clone in directory '%s'", cloneDir)
	}
	options.Dir = cloneDir
	return nil
}

// DiscoverGit checks if there is a git clone or prompts the user to import it
func (options *ImportOptions) DiscoverGit() error {
	surveyOpts := survey.WithStdio(options.In, options.Out, options.Err)
	if !options.DisableDotGitSearch {
		root, gitConf, err := options.Git().FindGitConfigDir(options.Dir)
		if err != nil {
			return err
		}
		if root != "" {
			if root != options.Dir {
				log.Infof("Importing from directory %s as we found a .git folder there\n", root)
			}
			options.Dir = root
			options.GitConfDir = gitConf
			return nil
		}
	}

	dir := options.Dir
	if dir == "" {
		return fmt.Errorf("no directory specified")
	}

	// lets prompt the user to initialise the Git repository
	if !options.BatchMode {
		log.Infof("The directory %s is not yet using git\n", util.ColorInfo(dir))
		flag := false
		prompt := &survey.Confirm{
			Message: "Would you like to initialise git now?",
			Default: true,
		}
		err := survey.AskOne(prompt, &flag, nil, surveyOpts)
		if err != nil {
			return err
		}
		if !flag {
			return fmt.Errorf("please initialise git yourself then try again")
		}
	}
	options.InitialisedGit = true
	err := options.Git().Init(dir)
	if err != nil {
		return err
	}
	options.GitConfDir = filepath.Join(dir, ".git", "config")
	err = options.DefaultGitIgnore()
	if err != nil {
		return err
	}
	err = options.Git().Add(dir, ".gitignore")
	if err != nil {
		return err
	}
	err = options.Git().Add(dir, "*")
	if err != nil {
		return err
	}

	err = options.Git().Status(dir)
	if err != nil {
		return err
	}

	message := options.ImportGitCommitMessage
	if message == "" {
		if options.BatchMode {
			message = "Initial import"
		} else {
			messagePrompt := &survey.Input{
				Message: "Commit message: ",
				Default: "Initial import",
			}
			err = survey.AskOne(messagePrompt, &message, nil, surveyOpts)
			if err != nil {
				return err
			}
		}
	}
	err = options.Git().CommitIfChanges(dir, message)
	if err != nil {
		return err
	}
	log.Infof("\nGit repository created\n")
	return nil
}

// DefaultGitIgnore creates a default .gitignore
func (options *ImportOptions) DefaultGitIgnore() error {
	name := filepath.Join(options.Dir, ".gitignore")
	exists, err := util.FileExists(name)
	if err != nil {
		return err
	}
	if !exists {
		data := []byte(defaultGitIgnoreFile)
		err = ioutil.WriteFile(name, data, DefaultWritePermissions)
		if err != nil {
			return fmt.Errorf("failed to write %s due to %s", name, err)
		}
	}
	return nil
}

// DiscoverRemoteGitURL finds the git url by looking in the directory
// and looking for a .git/config file
func (options *ImportOptions) DiscoverRemoteGitURL() error {
	gitConf := options.GitConfDir
	if gitConf == "" {
		return fmt.Errorf("no GitConfDir defined")
	}
	cfg := gitcfg.NewConfig()
	data, err := ioutil.ReadFile(gitConf)
	if err != nil {
		return fmt.Errorf("failed to load %s due to %s", gitConf, err)
	}

	err = cfg.Unmarshal(data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal %s due to %s", gitConf, err)
	}
	remotes := cfg.Remotes
	if len(remotes) == 0 {
		return nil
	}
	url := options.Git().GetRemoteUrl(cfg, "origin")
	if url == "" {
		url = options.Git().GetRemoteUrl(cfg, "upstream")
		if url == "" {
			url, err = options.pickRemoteURL(cfg)
			if err != nil {
				return err
			}
		}
	}
	if url != "" {
		options.RepoURL = url
	}
	return nil
}

func (options *ImportOptions) doImport() error {
	gitURL := options.RepoURL
	gitProvider := options.GitProvider
	if gitProvider == nil {
		p, err := options.gitProviderForURL(gitURL, "user name to register webhook")
		if err != nil {
			return err
		}
		gitProvider = p
	}

	authConfigSvc, err := options.CreateGitAuthConfigService()
	if err != nil {
		return err
	}
	jenkinsfile := options.Jenkinsfile
	if jenkinsfile == "" {
		jenkinsfile = jenkins.DefaultJenkinsfile
	}

	err = options.ensureDockerRepositoryExists()
	if err != nil {
		return err
	}

	isProw, err := options.isProw()
	if err != nil {
		return err
	}
	if isProw {
		// register the webhook
		err = options.createWebhookProw(gitURL, gitProvider)
		if err != nil {
			return err
		}
		return options.addProwConfig(gitURL)
	}

	return options.ImportProject(gitURL, options.Dir, jenkinsfile, options.BranchPattern, options.Credentials, false, gitProvider, authConfigSvc, false, options.BatchMode)
}

func (options *ImportOptions) addProwConfig(gitURL string) error {
	gitInfo, err := gits.ParseGitURL(gitURL)
	if err != nil {
		return err
	}
	repo := gitInfo.Organisation + "/" + gitInfo.Name
	err = prow.AddApplication(options.KubeClientCached, []string{repo}, options.currentNamespace, options.DraftPack)
	if err != nil {
		return err
	}

	// todo lets create a Knative build to auto optionally auto trigger initial release

	options.logImportedProject(false, gitInfo)

	return nil
}

// ensureDockerRepositoryExists for some kinds of container registry we need to pre-initialise its use such as for ECR
func (options *ImportOptions) ensureDockerRepositoryExists() error {
	orgName := options.getOrganisationOrCurrentUser()
	appName := options.AppName
	if orgName == "" {
		log.Warnf("Missing organisation name!\n")
		return nil
	}
	if appName == "" {
		log.Warnf("Missing application name!\n")
		return nil
	}
	kubeClient, curNs, err := options.KubeClient()
	if err != nil {
		return err
	}
	ns, _, err := kube.GetDevNamespace(kubeClient, curNs)
	if err != nil {
		return err
	}

	cm, err := kubeClient.CoreV1().ConfigMaps(ns).Get(kube.ConfigMapJenkinsDockerRegistry, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Could not find ConfigMap %s in namespace %s: %s", kube.ConfigMapJenkinsDockerRegistry, ns, err)
	}
	if cm.Data != nil {
		dockerRegistry := cm.Data["docker.registry"]
		if dockerRegistry != "" {
			if strings.HasSuffix(dockerRegistry, ".amazonaws.com") && strings.Index(dockerRegistry, ".ecr.") > 0 {
				return amazon.LazyCreateRegistry(orgName, appName)
			}
		}
	}
	return nil
}

// ReplacePlaceholders replaces Git server name, git org, and docker registry org placeholders
func (options *ImportOptions) ReplacePlaceholders(gitServerName, gitOrg, dockerRegistryOrg string) error {
	gitOrg = kube.ToValidName(strings.ToLower(gitOrg))
	log.Infof("replacing placeholders in directory %s\n", options.Dir)
	log.Infof("app name: %s, git server: %s, org: %s, Docker registry org: %s\n", options.AppName, gitServerName, gitOrg, dockerRegistryOrg)

	ignore, err := gitignore.NewRepository(options.Dir)
	if err != nil {
		return err
	}

	if err := filepath.Walk(options.Dir, func(f string, fi os.FileInfo, err error) error {
		relPath, _ := filepath.Rel(options.Dir, f)
		match := ignore.Relative(relPath, fi.IsDir())
		matchIgnore := match != nil && match.Ignore() //Defaults to including if match == nil

		if fi.IsDir() {
			if matchIgnore || fi.Name() == ".git" {
				log.Infof("skipping directory %q\n", f)
				return filepath.SkipDir
			}
			return nil
		}

		// Don't process nor follow symlinks
		if (fi.Mode() & os.ModeSymlink) == os.ModeSymlink {
			log.Infof("skipping symlink file %q\n", f)
			return nil
		}

		if !matchIgnore {
			input, err := ioutil.ReadFile(f)
			if err != nil {
				log.Errorf("failed to read file %s: %v", f, err)
				return err
			}

			lines := strings.Split(string(input), "\n")

			for i, line := range lines {
				line = strings.Replace(line, PlaceHolderAppName, strings.ToLower(options.AppName), -1)
				line = strings.Replace(line, PlaceHolderGitProvider, strings.ToLower(gitServerName), -1)
				line = strings.Replace(line, PlaceHolderOrg, strings.ToLower(gitOrg), -1)
				line = strings.Replace(line, PlaceHolderDockerRegistryOrg, strings.ToLower(dockerRegistryOrg), -1)
				lines[i] = line
			}
			output := strings.Join(lines, "\n")
			err = ioutil.WriteFile(f, []byte(output), 0644)
			if err != nil {
				log.Errorf("failed to write file %s: %v", f, err)
				return err
			}
		}
		return nil

	}); err != nil {
		return fmt.Errorf("error replacing placeholders %v", err)
	}

	return nil
}

func (options *ImportOptions) addAppNameToGeneratedFile(filename, field, value string) error {
	dir := filepath.Join(options.Dir, "charts", options.AppName)
	file := filepath.Join(dir, filename)
	exists, err := util.FileExists(file)
	if err != nil {
		return err
	}
	if !exists {
		// no file so lets ignore this
		return nil
	}
	input, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	lines := strings.Split(string(input), "\n")

	for i, line := range lines {
		if strings.Contains(line, field) {
			lines[i] = fmt.Sprintf("%s%s", field, value)
		}
	}
	output := strings.Join(lines, "\n")
	err = ioutil.WriteFile(file, []byte(output), 0644)
	if err != nil {
		return err
	}
	return nil
}

func (options *ImportOptions) checkChartmuseumCredentialExists() error {
	name := jenkins.DefaultJenkinsCredentialsPrefix + jenkins.Chartmuseum
	_, err := options.Jenkins.GetCredential(name)

	if err != nil {
		secret, err := options.KubeClientCached.CoreV1().Secrets(options.currentNamespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error getting %s secret %v", name, err)
		}

		data := secret.Data
		username := string(data["BASIC_AUTH_USER"])
		password := string(data["BASIC_AUTH_PASS"])

		err = options.retry(3, 10*time.Second, func() (err error) {
			return options.Jenkins.CreateCredential(name, username, password)
		})

		if err != nil {
			return fmt.Errorf("error creating Jenkins credential %s %v", name, err)
		}
	}
	return nil
}

func (options *ImportOptions) renameChartToMatchAppName() error {
	var oldChartsDir string
	dir := options.Dir
	chartsDir := filepath.Join(dir, "charts")
	files, err := ioutil.ReadDir(chartsDir)
	if err != nil {
		return fmt.Errorf("error matching a Jenkins X draft pack name with chart folder %v", err)
	}
	for _, fi := range files {
		if fi.IsDir() {
			name := fi.Name()
			// TODO we maybe need to try check if the sub dir named after the build pack matches first?
			if name != "preview" && name != ".git" {
				oldChartsDir = filepath.Join(chartsDir, name)
				break
			}
		}
	}
	if oldChartsDir != "" {
		// chart expects folder name to be the same as app name
		newChartsDir := filepath.Join(dir, "charts", options.AppName)

		exists, err := util.FileExists(oldChartsDir)
		if err != nil {
			return err
		}
		if exists {
			err = util.RenameDir(oldChartsDir, newChartsDir, false)
			if err != nil {
				return fmt.Errorf("error renaming %s to %s, %v", oldChartsDir, newChartsDir, err)
			}
			_, err = os.Stat(newChartsDir)
			if err != nil {
				return err
			}
		}
		// now update the chart.yaml
		err = options.addAppNameToGeneratedFile("Chart.yaml", "name: ", options.AppName)
		if err != nil {
			return err
		}
	}
	return nil
}

func (options *ImportOptions) fixDockerIgnoreFile() error {
	filename := filepath.Join(options.Dir, ".dockerignore")
	exists, err := util.FileExists(filename)
	if err == nil && exists {
		data, err := ioutil.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("Failed to load %s: %s", filename, err)
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) == "Dockerfile" {
				lines = append(lines[:i], lines[i+1:]...)
				text := strings.Join(lines, "\n")
				err = ioutil.WriteFile(filename, []byte(text), DefaultWritePermissions)
				if err != nil {
					return err
				}
				log.Infof("Removed old `Dockerfile` entry from %s\n", util.ColorInfo(filename))
			}
		}
	}
	return nil
}

// CreateProwOwnersFile creates an OWNERS file in the root of the project assigning the current Git user as an approver and a reviewer. If the file already exists, does nothing.
func (options *ImportOptions) CreateProwOwnersFile() error {
	filename := filepath.Join(options.Dir, "OWNERS")
	exists, err := util.FileExists(filename)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if options.GitUserAuth != nil && options.GitUserAuth.Username != "" {
		data := struct {
			Approvers []string `yaml:"approvers"`
			Reviewers []string `yaml:"reviewers"`
		}{
			[]string{options.GitUserAuth.Username},
			[]string{options.GitUserAuth.Username},
		}
		yaml, err := yaml.Marshal(&data)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(filename, []byte(yaml), 0644)
		if err != nil {
			return err
		}
		return nil
	}
	return errors.New("GitUserAuth.Username not set")
}

// CreateProwOwnersAliasesFile creates an OWNERS_ALIASES file in the root of the project assigning the current Git user as an approver and a reviewer.
func (options *ImportOptions) CreateProwOwnersAliasesFile() error {
	filename := filepath.Join(options.Dir, "OWNERS_ALIASES")
	exists, err := util.FileExists(filename)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if options.GitUserAuth == nil {
		return errors.New("option GitUserAuth not set")
	}
	gitUser := options.GitUserAuth.Username
	if gitUser != "" {
		data := struct {
			Aliases       []string `yaml:"aliases"`
			BestApprovers []string `yaml:"best-approvers"`
			BestReviewers []string `yaml:"best-reviewers"`
		}{
			[]string{gitUser},
			[]string{gitUser},
			[]string{gitUser},
		}
		yaml, err := yaml.Marshal(&data)
		if err != nil {
			return err
		}
		return ioutil.WriteFile(filename, []byte(yaml), 0644)
	}
	return errors.New("GitUserAuth.Username not set")
}

func (options *ImportOptions) fixMaven() error {
	if options.DisableMaven {
		return nil
	}
	dir := options.Dir
	pomName := filepath.Join(dir, "pom.xml")
	exists, err := util.FileExists(pomName)
	if err != nil {
		return err
	}
	if exists {
		err = options.installMavenIfRequired()
		if err != nil {
			return err
		}

		// lets ensure the mvn plugins are ok
		out, err := options.getCommandOutput(dir, "mvn", "io.jenkins.updatebot:updatebot-maven-plugin:RELEASE:plugin", "-Dartifact=maven-deploy-plugin", "-Dversion="+minimumMavenDeployVersion)
		if err != nil {
			return fmt.Errorf("Failed to update maven plugin: %s output: %s", err, out)
		}
		if !options.DryRun {
			err = options.Git().Add(dir, "pom.xml")
			if err != nil {
				return err
			}
			err = options.Git().CommitIfChanges(dir, "fix:(plugins) use a better version of maven deploy plugin")
			if err != nil {
				return err
			}
		}

		// lets ensure the probe paths are ok
		out, err = options.getCommandOutput(dir, "mvn", "io.jenkins.updatebot:updatebot-maven-plugin:RELEASE:chart")
		if err != nil {
			return fmt.Errorf("Failed to update chart: %s output: %s", err, out)
		}
		if !options.DryRun {
			exists, err := util.FileExists(filepath.Join(dir, "charts"))
			if err != nil {
				return err
			}
			if exists {
				err = options.Git().Add(dir, "charts")
				if err != nil {
					return err
				}
				err = options.Git().CommitIfChanges(dir, "fix:(chart) fix up the probe path")
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (options *ImportOptions) DefaultsFromTeamSettings() error {
	settings, err := options.TeamSettings()
	if err != nil {
		return err
	}
	if options.Organisation == "" {
		options.Organisation = settings.Organisation
	}
	if options.DockerRegistryOrg == "" {
		options.DockerRegistryOrg = settings.DockerRegistryOrg
	}
	if options.GitRepositoryOptions.ServerURL == "" {
		options.GitRepositoryOptions.ServerURL = settings.GitServer
	}
	options.GitRepositoryOptions.Private = settings.GitPrivate || options.GitRepositoryOptions.Private
	options.PipelineServer = settings.GitServer
	options.PipelineUserName = settings.PipelineUsername
	return nil
}

func (o *ImportOptions) allDraftPacks() ([]string, error) {
	// lets make sure we have the latest draft packs
	initOpts := InitOptions{
		CommonOptions: o.CommonOptions,
	}
	log.Info("Getting latest packs ...\n")
	dir, err := initOpts.initBuildPacks()
	if err != nil {
		return nil, err
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0)
	for _, f := range files {
		if f.IsDir() {
			result = append(result, f.Name())
		}
	}
	return result, err

}
