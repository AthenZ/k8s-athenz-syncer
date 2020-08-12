// +build e2e

package framework

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/yahoo/athenz/clients/go/zms"
	athenzClientset "github.com/yahoo/k8s-athenz-syncer/pkg/client/clientset/versioned"
	athenzInformer "github.com/yahoo/k8s-athenz-syncer/pkg/client/informers/externalversions/athenz/v1"
	"github.com/yahoo/k8s-athenz-syncer/pkg/cr"
	"github.com/yahoo/k8s-athenz-syncer/pkg/log"
	"github.com/yahoo/k8s-athenz-syncer/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type Framework struct {
	K8sClient       kubernetes.Interface
	ZMSClient       zms.ZMSClient
	CRClient        cr.CRUtil
	RoleDomain      string
	RoleName        string
	TrustDomain     string
	TrustRole       string
	NamespaceDomain string
	MyUtil          util.Util
}

var Global *Framework

// Setup() create necessary clients for tests
func setup(stopCh <-chan struct{}) error {
	// config
	kubeconfig := flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	inClusterConfig := flag.Bool("inClusterConfig", true, "Set to true to use in cluster config.")
	key := flag.String("key", "/var/run/athenz/service.key.pem", "Athenz private key file")
	cert := flag.String("cert", "/var/run/athenz/service.cert.pem", "Athenz certificate file")
	zmsURL := flag.String("zms-url", "", "Athenz ZMS API URL")
	logLoc := flag.String("log-location", "/var/log/k8s-athenz-syncer.log", "log location")
	logMode := flag.String("log-mode", "info", "logger mode")
	roleTestDomain := flag.String("e2e-test1-domain", "k8s.omega.stage.kube-test", "athenz domain used for e2e test 1")
	roleName := flag.String("e2e-test1-role", "syncer-e2e", "athenz role used for e2e test 1")
	trustDomain := flag.String("e2e-test2-domain", "prod-eng.omega.acceptancetest.trust-domain", "athenz trust domain used for e2e test 2")
	trustRoleName := flag.String("e2e-test2-role", "test-trustrole", "athenz trust role used for e2e test 2")
	namespaceDomain := flag.String("e2e-test3-domain", "prod-eng.omega.acceptancetest.test-domain", "athenz domain used for e2e test 3")
	flag.Parse()

	// init logger
	log.InitLogger(*logLoc, *logMode)

	// if kubeconfig is empty
	if *kubeconfig == "" {
		if *inClusterConfig {
			emptystr := ""
			kubeconfig = &emptystr
		} else {
			if home := util.HomeDir(); home != "" {
				defaultconfig := filepath.Join(home, ".kube", "config")
				kubeconfig = &defaultconfig
			}
		}
	}
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return err
	}

	// set up k8s client
	k8sclient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error("Failed to create k8s client")
		return err
	}

	// set up athenzdomains client
	athenzClient, err := athenzClientset.NewForConfig(config)
	if err != nil {
		log.Error("Failed to create athenz domains client")
		return err
	}
	// set up cr informer to get athenzdomains resources
	crIndexInformer := athenzInformer.NewAthenzDomainInformer(athenzClient, 0, cache.Indexers{})

	go crIndexInformer.Run(stopCh)
	// do the initial synchronization (one time) to populate resources
	if !cache.WaitForCacheSync(stopCh, crIndexInformer.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Error syncing cache"))
		return fmt.Errorf("Error syncing cache")
	}
	log.Info("athenz domains cache sync complete")

	crutil := cr.NewCRUtil(athenzClient, crIndexInformer)

	// set up zms client
	zmsclient, err := setupZMSClient(*key, *cert, *zmsURL)
	if err != nil {
		log.Error("Failed to create zms client")
		return err
	}
	adminDomain := ""
	systemNS := []string{}
	domainUtil := util.NewUtil(adminDomain, systemNS)

	Global = &Framework{
		K8sClient:       k8sclient,
		ZMSClient:       *zmsclient,
		CRClient:        *crutil,
		RoleDomain:      *roleTestDomain,
		RoleName:        *roleName,
		TrustDomain:     *trustDomain,
		TrustRole:       *trustRoleName,
		NamespaceDomain: *namespaceDomain,
		MyUtil:          *domainUtil,
	}
	return nil
}

// set up zms client, skipping cert reloader
func setupZMSClient(key string, cert string, zmsURL string) (*zms.ZMSClient, error) {
	clientCert, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, fmt.Errorf("Unable to formulate clientCert from key and cert bytes, error: %v", err)
	}
	config := &tls.Config{}
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0] = clientCert
	transport := &http.Transport{
		TLSClientConfig: config,
	}
	client := zms.NewClient(zmsURL, transport)
	return &client, nil
}

// teardown Framework
func teardown() error {
	f := Global
	domain := zms.DomainName(f.RoleDomain)
	roleName := zms.EntityName(f.RoleName)
	err := f.ZMSClient.DeleteRole(domain, roleName, "")
	if err != nil {
		log.Error("Unable to delete test1 role")
		return err
	}
	trustroleName := zms.EntityName(f.TrustRole)
	err = f.ZMSClient.DeleteRole(domain, trustroleName, "")
	if err != nil {
		log.Error("Unable to delete test2 role")
		return err
	}
	err = f.CRClient.RemoveAthenzDomain(f.TrustDomain)
	if err != nil {
		log.Error("Unable to remove created athenzdomains")
		return err
	}
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := &metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}
	namespace := f.MyUtil.DomainToNamespace(f.NamespaceDomain)
	err = f.K8sClient.CoreV1().Namespaces().Delete(namespace, deleteOptions)
	if err != nil {
		log.Error("Unable to delete test namespace")
		return err
	}
	Global = nil
	log.Info("e2e teardown successfully")
	return nil
}
