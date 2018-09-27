package cmd_test

import (
	"os"
	"testing"

	"github.com/jenkins-x/jx/pkg/jx/cmd"
	cmd_mocks "github.com/jenkins-x/jx/pkg/jx/cmd/mocks"
	. "github.com/petergtz/pegomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kube_mocks "k8s.io/client-go/kubernetes/fake"

	//kube_mocks "k8s.io/client-go/kubernetes/fake"
	versiond_mocks "github.com/jenkins-x/jx/pkg/client/clientset/versioned/fake"
)

func TestJXClient(t *testing.T) {
	// mock factory
	factory := cmd_mocks.NewMockFactory()
	// mock versiond interface
	versiondInterface := versiond_mocks.NewSimpleClientset()
	// Override CreateJXClient to return mock versiond interface
	When(factory.CreateJXClient()).ThenReturn(versiondInterface, "jx-testing", nil)

	options := cmd.CommonOptions{
		Factory: factory,
		Out:     os.Stdout,
		Err:     os.Stderr,
	}

	interf, ns, err := options.JXClient()

	assert.NoError(t, err, "Should not error")
	assert.Equal(t, "jx-testing", ns)
	assert.Equal(t, versiondInterface, interf)
}

func TestJXClientAndDevNameSpace(t *testing.T) {
	// namespace fixture
	namespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jx-testing",
			Namespace: "jx-testing",
		},
	}
	// mock factory
	factory := cmd_mocks.NewMockFactory()
	// mock Kubernetes interface
	kubernetesInterface := kube_mocks.NewSimpleClientset(namespace)
	// Override CreateClient to return mock Kubernetes interface
	When(factory.CreateClient()).ThenReturn(kubernetesInterface, "jx-testing", nil)
	// mock versiond interface
	versiondInterface := versiond_mocks.NewSimpleClientset()
	// Override CreateJXClient to return mock versiond interface
	When(factory.CreateJXClient()).ThenReturn(versiondInterface, "jx-testing", nil)

	options := cmd.CommonOptions{
		Factory: factory,
		Out:     os.Stdout,
		Err:     os.Stderr,
	}

	interf, ns, err := options.JXClientAndDevNamespace()

	assert.NoError(t, err, "Should not error")
	assert.Equal(t, "jx-testing", ns)
	assert.Equal(t, versiondInterface, interf)
}
