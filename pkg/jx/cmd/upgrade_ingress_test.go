package cmd_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/jenkins-x/jx/pkg/jx/cmd"
	"github.com/jenkins-x/jx/pkg/kube"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testclient "k8s.io/client-go/kubernetes/fake"
)

type TestOptions struct {
	cmd.UpgradeIngressOptions
	Service *v1.Service
}

func (o *TestOptions) Setup() {
	o.UpgradeIngressOptions = cmd.UpgradeIngressOptions{
		CreateOptions: cmd.CreateOptions{
			CommonOptions: cmd.CommonOptions{
				KubeClientCached: testclient.NewSimpleClientset(),
			},
		},
		IngressConfig: kube.IngressConfig{
			Issuer: "letsencrypt-prod",
		},
		TargetNamespaces: []string{"test"},
	}

	o.Service = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Spec: v1.ServiceSpec{},
	}

	o.Service.Annotations = map[string]string{}
	o.Service.Annotations[kube.ExposeAnnotation] = "true"
}

func TestAnnotateNoExisting(t *testing.T) {
	t.Parallel()
	o := TestOptions{}
	o.Setup()

	_, err := o.KubeClientCached.CoreV1().Services("test").Create(o.Service)
	assert.NoError(t, err)

	err = o.CleanServiceAnnotations()
	assert.NoError(t, err)

	err = o.AnnotateExposedServicesWithCertManager()
	assert.NoError(t, err)

	rs, err := o.KubeClientCached.CoreV1().Services("test").Get("foo", metav1.GetOptions{})
	ingressAnnotations := rs.Annotations[kube.ExposeIngressAnnotation]

	assert.Equal(t, "certmanager.k8s.io/issuer: letsencrypt-prod", ingressAnnotations)
	assert.NoError(t, err)
}

func TestAnnotateWithExistingAnnotations(t *testing.T) {

	o := TestOptions{}
	o.Setup()

	o.Service.Annotations[kube.ExposeIngressAnnotation] = "foo: bar\nkubernetes.io/ingress.class: nginx\nnginx.ingress.kubernetes.io/proxy-body-size: 500m"

	_, err := o.KubeClientCached.CoreV1().Services("test").Create(o.Service)
	assert.NoError(t, err)

	err = o.CleanServiceAnnotations()
	assert.NoError(t, err)

	err = o.AnnotateExposedServicesWithCertManager()
	assert.NoError(t, err)

	rs, err := o.KubeClientCached.CoreV1().Services("test").Get("foo", metav1.GetOptions{})
	ingressAnnotations := rs.Annotations[kube.ExposeIngressAnnotation]

	assert.Equal(t, "foo: bar\nkubernetes.io/ingress.class: nginx\nnginx.ingress.kubernetes.io/proxy-body-size: 500m\ncertmanager.k8s.io/issuer: letsencrypt-prod", ingressAnnotations)
	assert.NoError(t, err)
}

func TestAnnotateWithExistingCertManagerAnnotation(t *testing.T) {
	t.Parallel()
	o := TestOptions{}
	o.Setup()

	o.Service.Annotations[kube.ExposeIngressAnnotation] = kube.CertManagerAnnotation + ": letsencrypt-staging"

	_, err := o.KubeClientCached.CoreV1().Services("test").Create(o.Service)
	assert.NoError(t, err)

	err = o.CleanServiceAnnotations()
	assert.NoError(t, err)

	err = o.AnnotateExposedServicesWithCertManager()
	assert.NoError(t, err)

	rs, err := o.KubeClientCached.CoreV1().Services("test").Get("foo", metav1.GetOptions{})
	ingressAnnotations := rs.Annotations[kube.ExposeIngressAnnotation]

	assert.Equal(t, "certmanager.k8s.io/issuer: letsencrypt-prod", ingressAnnotations)
	assert.NoError(t, err)
}

func TestCleanExistingExposecontrollerReources(t *testing.T) {
	t.Parallel()
	o := TestOptions{}
	o.Setup()

	cm := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "exposecontroller",
		},
	}
	_, err := o.KubeClientCached.CoreV1().ConfigMaps("test").Create(&cm)
	assert.NoError(t, err)
	o.CleanExposecontrollerReources("test")

	_, err = o.KubeClientCached.CoreV1().ConfigMaps("test").Get("exposecontroller", metav1.GetOptions{})
	assert.Error(t, err)
}

func TestCleanServiceAnnotations(t *testing.T) {
	t.Parallel()
	o := TestOptions{}
	o.Setup()

	o.Service.Annotations[kube.ExposeURLAnnotation] = "http://foo.bar"

	_, err := o.KubeClientCached.CoreV1().Services("test").Create(o.Service)
	assert.NoError(t, err)

	err = o.CleanServiceAnnotations()
	assert.NoError(t, err)

	rs, err := o.KubeClientCached.CoreV1().Services("test").Get("foo", metav1.GetOptions{})

	assert.Empty(t, rs.Annotations[kube.ExposeURLAnnotation])
	assert.NoError(t, err)
}

func TestRealJenkinsService(t *testing.T) {
	t.Parallel()
	filename := filepath.Join(".", "test_data", "upgrade_ingress", "service.yaml")
	serviceYaml, err := ioutil.ReadFile(filename)
	assert.NoError(t, err)

	var svc *v1.Service
	err = yaml.Unmarshal(serviceYaml, &svc)
	assert.NoError(t, err)

	o := TestOptions{}
	o.Setup()

	o.Service = svc

	_, err = o.KubeClientCached.CoreV1().Services("test").Create(o.Service)
	assert.NoError(t, err)

	err = o.CleanServiceAnnotations()
	assert.NoError(t, err)

	err = o.AnnotateExposedServicesWithCertManager()
	assert.NoError(t, err)

	rs, err := o.KubeClientCached.CoreV1().Services("test").Get("jenkins", metav1.GetOptions{})
	ingressAnnotations := rs.Annotations[kube.ExposeIngressAnnotation]

	assert.Equal(t, "kubernetes.io/ingress.class: nginx\nnginx.ingress.kubernetes.io/proxy-body-size: 500m\ncertmanager.k8s.io/issuer: letsencrypt-prod", ingressAnnotations)
	assert.NoError(t, err)
}
