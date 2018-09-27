package cmd_test

import (
	"testing"

	expect "github.com/Netflix/go-expect"
	gojenkins "github.com/jenkins-x/golang-jenkins"
	jenkins_mocks "github.com/jenkins-x/golang-jenkins/mocks"
	versiond_mocks "github.com/jenkins-x/jx/pkg/client/clientset/versioned/fake"
	git_mocks "github.com/jenkins-x/jx/pkg/gits/mocks"
	"github.com/jenkins-x/jx/pkg/jx/cmd"
	cmd_mocks "github.com/jenkins-x/jx/pkg/jx/cmd/mocks"
	cmd_mock_matchers "github.com/jenkins-x/jx/pkg/jx/cmd/mocks/matchers"
	"github.com/jenkins-x/jx/pkg/tests"
	. "github.com/petergtz/pegomock"
	"github.com/stretchr/testify/assert"
	"gopkg.in/AlecAivazis/survey.v1/core"
	"k8s.io/api/core/v1"
	apiextentions_mocks "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kube_mocks "k8s.io/client-go/kubernetes/fake"
)

func init() {
	// disable color output for all prompts to simplify testing
	core.DisableColor = true
}

func TestImportProject(t *testing.T) {
	// namespace fixture
	namespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "jx-testing",
		},
	}

	jenkinsConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jenkins",
			Namespace: "jx-testing",
		},
	}

	// mock factory
	factory := cmd_mocks.NewMockFactory()

	// mock terminal
	c, state, term := tests.NewTerminal(t)

	// Test interactive IO
	donec := make(chan struct{})
	go func() {
		defer close(donec)
		c.ExpectString("Do you wish to use jx-testing-user as the user name for the Jenkins Pipeline")
		c.SendLine("Y")
		c.ExpectEOF()
	}()

	// mock Kubernetes interface
	kubernetesInterface := kube_mocks.NewSimpleClientset(namespace, jenkinsConfigMap)
	// Override CreateClient to return mock Kubernetes interface
	When(factory.CreateClient()).ThenReturn(kubernetesInterface, "jx-testing", nil)

	// mock apiExtensions interface
	apiextensionsInterface := apiextentions_mocks.NewSimpleClientset()
	// Override CreateApiExtensionsClient to return mock apiextensions interface
	When(factory.CreateApiExtensionsClient()).ThenReturn(apiextensionsInterface, nil)

	// mock versiond interface
	versiondInterface := versiond_mocks.NewSimpleClientset()
	// Override CreateJXClient to return mock versiond interface
	When(factory.CreateJXClient()).ThenReturn(versiondInterface, "jx-testing", nil)

	// mock Jenkins client
	jenkinsClientInterface := jenkins_mocks.NewMockJenkinsClient()
	When(factory.CreateJenkinsClient(cmd_mock_matchers.AnyKubernetesInterface(), AnyString(), cmd_mock_matchers.AnyTerminalFileReader(), cmd_mock_matchers.AnyTerminalFileWriter(), cmd_mock_matchers.AnyIoWriter())).ThenReturn(jenkinsClientInterface, nil)

	jenkinsJob := gojenkins.Job{Class: "com.cloudbees.hudson.plugins.folder.Folder"}

	When(jenkinsClientInterface.GetJob(AnyString())).ThenReturn(jenkinsJob, nil)

	o := &cmd.ImportOptions{
		CommonOptions: cmd.CommonOptions{
			Factory: factory,
			In:      term.In,
			Out:     term.Out,
			Err:     term.Err,
		},
	}

	gitURL := "https://github.com/jx-testing-user/jx-testing-env"
	dir := ""
	jenkinsFile := ""
	branchPattern := ""
	credentials := ""
	failIfExists := false
	gitProviderInterface := git_mocks.NewMockGitProvider()
	authConfigSvc := tests.CreateAuthConfigService()
	isEnvironment := true
	batchMode := false

	err := o.ImportProject(
		gitURL,
		dir,
		jenkinsFile,
		branchPattern,
		credentials,
		failIfExists,
		gitProviderInterface,
		authConfigSvc,
		isEnvironment,
		batchMode,
	)

	// Close the slave end of the pty, and read the remaining bytes from the master end.
	c.Tty().Close()
	<-donec

	assert.NoError(t, err, "Should not error")

	// Dump the terminal's screen.
	t.Logf(expect.StripTrailingEmptyLines(state.String()))
}
