package pki

import (
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	hyperv1 "github.com/openshift/hypershift/api/v1beta1"
	"github.com/openshift/hypershift/control-plane-operator/controllers/hostedcontrolplane/manifests"
	"github.com/openshift/hypershift/support/certs"
	"github.com/openshift/hypershift/support/config"
	"github.com/openshift/hypershift/support/util"
)

const (
	// Service signer secret keys
	ServiceSignerPrivateKey = "service-account.key"
	ServiceSignerPublicKey  = "service-account.pub"
)

func ReconcileKASServerCertSecret(secret, ca *corev1.Secret, ownerRef config.OwnerRef, externalAPIAddress, internalAPIAddress string, serviceCIDRs []string) error {
	svc := manifests.KubeAPIServerService(secret.Namespace)
	svcAddresses := make([]string, 0)

	for _, serviceCIDR := range serviceCIDRs {
		serviceIP, err := util.FirstUsableIP(serviceCIDR)
		if err != nil {
			return fmt.Errorf("cannot get the first usable IP from CIDR %s: %w", serviceIP, err)
		}
		svcAddresses = append(svcAddresses, serviceIP)
	}

	dnsNames := []string{
		"localhost",
		"kubernetes",
		"kubernetes.default",
		"kubernetes.default.svc",
		"kubernetes.default.svc.cluster.local",
		svc.Name,
		fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace),
	}
	apiServerIPs := []string{
		"127.0.0.1",
		"0:0:0:0:0:0:0:1",
	}
	apiServerIPs = append(apiServerIPs, svcAddresses...)

	if isNumericIP(externalAPIAddress) {
		apiServerIPs = append(apiServerIPs, externalAPIAddress)
	} else {
		dnsNames = append(dnsNames, externalAPIAddress)
	}
	if isNumericIP(internalAPIAddress) {
		apiServerIPs = append(apiServerIPs, internalAPIAddress)
	} else {
		dnsNames = append(dnsNames, internalAPIAddress)
	}
	return reconcileSignedCertWithAddresses(secret, ca, ownerRef, "kubernetes", []string{"kubernetes"}, X509UsageServerAuth, dnsNames, apiServerIPs)
}

func ReconcileKASKubeletClientCertSecret(secret, ca *corev1.Secret, ownerRef config.OwnerRef) error {
	return reconcileSignedCert(secret, ca, ownerRef, "system:kube-apiserver", []string{"kubernetes"}, X509UsageClientAuth)
}

func ReconcileKASMachineBootstrapClientCertSecret(secret, ca *corev1.Secret, ownerRef config.OwnerRef) error {
	return reconcileSignedCert(secret, ca, ownerRef, "system:serviceaccount:openshift-machine-config-operator:node-bootstrapper", []string{"system:serviceaccounts:openshift-machine-config-operator", "system:serviceaccounts"}, X509UsageClientAuth)
}

func ReconcileKASAggregatorCertSecret(secret, ca *corev1.Secret, ownerRef config.OwnerRef) error {
	return reconcileSignedCert(secret, ca, ownerRef, "system:openshift-aggregator", []string{"kubernetes"}, X509UsageClientAuth)
}

func ReconcileKubeSchedulerClientCertSecret(secret, ca *corev1.Secret, ownerRef config.OwnerRef) error {
	return reconcileSignedCert(secret, ca, ownerRef, "system:kube-scheduler", []string{"kubernetes"}, X509UsageClientAuth)
}

func ReconcileKubeControllerManagerClientCertSecret(secret, ca *corev1.Secret, ownerRef config.OwnerRef) error {
	return reconcileSignedCert(secret, ca, ownerRef, "system:kube-controller-manager", []string{"kubernetes"}, X509UsageClientAuth)
}

func ReconcileSystemAdminClientCertSecret(secret, ca *corev1.Secret, ownerRef config.OwnerRef) error {
	return reconcileSignedCert(secret, ca, ownerRef, "system:admin", []string{"system:masters"}, X509UsageClientAuth)
}

func ReconcileServiceAccountKubeconfig(secret, csrSigner *corev1.Secret, ca *corev1.ConfigMap, hcp *hyperv1.HostedControlPlane, serviceAccountNamespace, serviceAccountName string) error {
	cn := serviceaccount.MakeUsername(serviceAccountNamespace, serviceAccountName)
	if err := reconcileSignedCert(secret, csrSigner, config.OwnerRef{}, cn, serviceaccount.MakeGroupNames(serviceAccountNamespace), X509UsageClientAuth); err != nil {
		return fmt.Errorf("failed to reconcile serviceaccount client cert: %w", err)
	}
	svcURL := inClusterKASURL(hcp.Namespace, util.InternalAPIPortWithDefault(hcp, config.DefaultAPIServerPort))

	return ReconcileKubeConfig(secret, secret, ca, svcURL, "", manifests.KubeconfigScopeLocal, config.OwnerRef{})
}

func isNumericIP(s string) bool {
	return net.ParseIP(s) != nil
}

func ReconcileKubeConfig(secret, cert *corev1.Secret, ca *corev1.ConfigMap, url string, key string, scope manifests.KubeconfigScope, ownerRef config.OwnerRef) error {
	ownerRef.ApplyTo(secret)
	caPEM := ca.Data[certs.CASignerCertMapKey]
	crtBytes, keyBytes := cert.Data[corev1.TLSCertKey], cert.Data[corev1.TLSPrivateKeyKey]
	kubeCfgBytes, err := generateKubeConfig(url, crtBytes, keyBytes, []byte(caPEM))
	if err != nil {
		return fmt.Errorf("failed to generate kubeconfig: %w", err)
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	if key == "" {
		key = util.KubeconfigKey
	}
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels[manifests.KubeconfigScopeLabel] = string(scope)
	secret.Data[key] = kubeCfgBytes
	return nil
}

func generateKubeConfig(url string, crtBytes, keyBytes, caBytes []byte) ([]byte, error) {
	kubeCfg := clientcmdapi.Config{
		Kind:       "Config",
		APIVersion: "v1",
	}
	kubeCfg.Clusters = map[string]*clientcmdapi.Cluster{
		"cluster": {
			Server:                   url,
			CertificateAuthorityData: caBytes,
		},
	}
	kubeCfg.AuthInfos = map[string]*clientcmdapi.AuthInfo{
		"admin": {
			ClientCertificateData: crtBytes,
			ClientKeyData:         keyBytes,
		},
	}
	kubeCfg.Contexts = map[string]*clientcmdapi.Context{
		"admin": {
			Cluster:   "cluster",
			AuthInfo:  "admin",
			Namespace: "default",
		},
	}
	kubeCfg.CurrentContext = "admin"
	return clientcmd.Write(kubeCfg)
}

func inClusterKASURL(namespace string, apiServerPort int32) string {
	return fmt.Sprintf("https://%s:%d", manifests.KubeAPIServerServiceName, apiServerPort)
}
