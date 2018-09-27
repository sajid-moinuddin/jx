package cmd_test

import (
	"testing"

	"github.com/jenkins-x/jx/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx/pkg/gits"
	"github.com/jenkins-x/jx/pkg/helm"
	"github.com/jenkins-x/jx/pkg/jx/cmd"
	"github.com/jenkins-x/jx/pkg/kube"
	"github.com/jenkins-x/jx/pkg/tests"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestEnvironmentRoleBinding(t *testing.T) {
	t.Parallel()
	o := &cmd.ControllerRoleOptions{
		NoWatch: true,
	}
	roleBindingName := "env-role-bindings"
	roleName := "myrole"
	roleNameWithLabel := "myroleWithLabel"
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: "jx",
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get", "watch", "list"},
				APIGroups: []string{""},
				Resources: []string{"configmaps", "pods", "services"},
			},
		},
	}

	label := make(map[string]string)
	label[kube.LabelKind] = kube.ValueKindEnvironmentRole
	roleWithLabel := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleNameWithLabel,
			Namespace: "jx",
			Labels:    label,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get", "watch", "list"},
				APIGroups: []string{""},
				Resources: []string{"configmaps", "pods", "services"},
			},
		},
	}

	envRoleBinding := &v1.EnvironmentRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: "jx",
		},
		Spec: v1.EnvironmentRoleBindingSpec{
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "jenkins",
					Namespace: "jx",
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     roleName,
			},
			Environments: []v1.EnvironmentFilter{
				{
					Includes: []string{"*"},
				},
			},
		},
	}

	cmd.ConfigureTestOptionsWithResources(&o.CommonOptions,
		[]runtime.Object{
			role,
			roleWithLabel,
		},
		[]runtime.Object{
			kube.NewPermanentEnvironment("staging"),
			kube.NewPermanentEnvironment("production"),
			kube.NewPreviewEnvironment("jx-jstrachan-demo96-pr-1"),
			kube.NewPreviewEnvironment("jx-jstrachan-another-pr-3"),
			envRoleBinding,
		},
		gits.NewGitCLI(),
		helm.NewHelmCLI("helm", helm.V2, "", true),
	)

	err := o.Run()
	assert.NoError(t, err)

	if err == nil {
		kubeClient, _, err := o.KubeClient()
		assert.NoError(t, err)
		if err == nil {
			nsNames := []string{"jx", "jx-staging", "jx-production", "jx-preview-jx-jstrachan-demo96-pr-1", "jx-preview-jx-jstrachan-another-pr-3"}
			for _, ns := range nsNames {
				roleBinding, err := kubeClient.RbacV1().RoleBindings(ns).Get(roleBindingName, metav1.GetOptions{})
				assert.NoError(t, err, "Failed to find RoleBinding in namespace %s for name %s", ns, roleBindingName)

				if roleBinding != nil && err == nil {
					assert.Equal(t, envRoleBinding.Spec.RoleRef, roleBinding.RoleRef,
						"RoleBinding.RoleRef for name %s in namespace %s", roleBindingName, ns)
				}

				r, err := kubeClient.RbacV1().Roles(ns).Get(roleName, metav1.GetOptions{})
				assert.NoError(t, err, "Failed to find Role in namespace %s for name %s", ns, roleName)

				if r != nil && err == nil {
					assert.Equal(t, role.Rules, r.Rules,
						"Role.Rules for name %s in namespace %s", roleBindingName, ns)
				}
				if util.StringMatchesPattern(ns, "jx") {
					jxClient, ns, err := o.JXClient()
					if err == nil {
						envRoleBindings, err := jxClient.JenkinsV1().EnvironmentRoleBindings(ns).Get(roleNameWithLabel, metav1.GetOptions{})
						if err != nil {
							assert.NotNil(t, envRoleBindings, "Role:"+roleNameWithLabel+" didn't get added to environment role bindings")
						}
					}
				}

			}
			if tests.IsDebugLog() {
				namespaces, err := kubeClient.CoreV1().Namespaces().List(metav1.ListOptions{})
				assert.NoError(t, err)
				if err == nil {
					for _, ns := range namespaces.Items {
						tests.Debugf("Has namespace %s\n", ns.Name)
					}
				}
			}
		}
	}
}
