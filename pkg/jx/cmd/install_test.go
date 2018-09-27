package cmd_test

import (
	"os"
	"path"
	"testing"

	"fmt"

	"github.com/jenkins-x/jx/pkg/jx/cmd"
	"github.com/jenkins-x/jx/pkg/util"

	//. "github.com/petergtz/pegomock"
	"github.com/stretchr/testify/assert"
)

func TestInstall(t *testing.T) {
	t.Parallel()
	testDir := path.Join("test_data", "install_cloud_environments_repo")
	_, err := os.Stat(testDir)
	assert.NoError(t, err)

	version, err := cmd.LoadVersionFromCloudEnvironmentsDir(testDir)
	assert.NoError(t, err)

	assert.Equal(t, "0.0.1436", version, "For Makefile in dir %s", testDir)
}

func TestGenerateProwSecret(t *testing.T) {
	fmt.Println(util.RandStringBytesMaskImprSrc(41))
}

func TestGetSafeUsername(t *testing.T) {
	t.Parallel()
	username := `Your active configuration is: [cloudshell-16392]
tutorial@bamboo-depth-206411.iam.gserviceaccount.com`
	assert.Equal(t, cmd.GetSafeUsername(username), "tutorial@bamboo-depth-206411.iam.gserviceaccount.com")

	username = `tutorial@bamboo-depth-206411.iam.gserviceaccount.com`
	assert.Equal(t, cmd.GetSafeUsername(username), "tutorial@bamboo-depth-206411.iam.gserviceaccount.com")
}

func TestInstallRun(t *testing.T) {
	// Create mocks...
	//factory := cmd_mocks.NewMockFactory()
	//kubernetesInterface := kube_mocks.NewSimpleClientset()
	//// Override CreateClient to return mock Kubernetes interface
	//When(factory.CreateClient()).ThenReturn(kubernetesInterface, "jx-testing", nil)

	//options := cmd.CreateInstallOptions(factory, os.Stdin, os.Stdout, os.Stderr)

	//err := options.Run()

	//assert.NoError(t, err, "Should not error")
}
