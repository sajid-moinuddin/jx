package cmd

import (
	"fmt"
	"time"

	"github.com/jenkins-x/jx/pkg/kube"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
)

func (o *CommonOptions) ensureCertmanager() error {
	log.Infof("Looking for %s deployment in namespace %s...\n", CertManagerDeployment, CertManagerNamespace)
	_, err := kube.GetDeploymentPods(o.KubeClientCached, CertManagerDeployment, CertManagerNamespace)
	if err != nil {
		ok := util.Confirm("CertManager deployment not found, shall we install it now?", true, "CertManager automatically configures Ingress rules with TLS using signed certificates from LetsEncrypt", o.In, o.Out, o.Err)
		if ok {

			values := []string{"rbac.create=true", "ingressShim.extraArgs='{--default-issuer-name=letsencrypt-staging,--default-issuer-kind=Issuer}'"}
			err = o.installChartOptions(InstallChartOptions{
				ReleaseName: "cert-manager",
				Chart:       "stable/cert-manager",
				Version:     "",
				Ns:          CertManagerNamespace,
				HelmUpdate:  true,
				SetValues:   values,
			})
			if err != nil {
				return fmt.Errorf("CertManager deployment failed: %v", err)
			}

			log.Info("waiting for CertManager deployment to be ready, this can take a few minutes\n")

			err = kube.WaitForDeploymentToBeReady(o.KubeClientCached, CertManagerDeployment, CertManagerNamespace, 10*time.Minute)
			if err != nil {
				return err
			}
		}
	}
	return err
}
