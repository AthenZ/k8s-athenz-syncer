package framework

import (
	"crypto/tls"
	"flag"
	"net/http"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/yahoo/athenz/clients/go/zms"
	athenzClientset "github.com/yahoo/k8s-athenz-syncer/pkg/client/clientset/versioned"
	r "github.com/yahoo/k8s-athenz-syncer/pkg/reloader"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Framework struct {
	K8sClient           kubernetes.Interface
	ZMSClient           zms.ZMSClient
	AthenzDomainsClient athenzClientset.Clientset
}

var Global *Framework

// Setup() create necessary clients for tests
func setup(stopCh <-chan struct{}) error {
	// config
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	inClusterConfig := flag.Bool("inClusterConfig", true, "Set to true to use in cluster config.")
	key := flag.String("key", "/var/run/athenz/service.key.pem", "Athenz private key file")
	cert := flag.String("cert", "/var/run/athenz/service.cert.pem", "Athenz certificate file")
	disableKeepAlives := flag.Bool("disable-keep-alives", true, "Disable keep alive for zms client")
	zmsURL := flag.String("zms-url", "", "Athenz ZMS API URL")
	logLoc := flag.String("log-location", "/var/log/k8s-athenz-syncer/k8s-athenz-syncer.log", "log location")
	logMode := flag.String("log-mode", "info", "logger mode")
	flag.Parse()

	// init logger
	log.InitLogger(*logLoc, *logMode)

	if *inClusterConfig {
		emptystr := ""
		kubeconfig = &emptystr
	}
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return err
	}

	// set up k8s client
	k8sclient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	// set up athenzdomains client
	athenzClient, err := athenzClientset.NewForConfig(config)
	if err != nil {
		return err
	}

	// set up zms client
	zmsclient, err := setupZMSClient(*key, *cert, *disableKeepAlives, *zmsURL, stopCh)
	if err != nil {
		return err
	}

	Global = &Framework{
		K8sClient:           k8sclient,
		ZMSClient:           zmsclient,
		AthenzDomainsClient: athenzClient,
	}
	return nil
}

func setupZMSClient(key string, cert string, disableKeepAlives bool, zmsURL string, stopCh <-chan struct{}) (zms.ZMSClient, error) {
	reloader, err := r.NewCertReloader(r.ReloadConfig{
		KeyFile:  key,
		CertFile: cert,
	}, stopCh)
	if err != nil {
		log.Panicf("Error occurred when creating new reloader. Error: %v", err)
	}
	config := &tls.Config{}
	config.GetClientCertificate = func(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
		return reloader.GetLatestCertificate(), nil
	}
	transport := &http.Transport{
		TLSClientConfig:   config,
		DisableKeepAlives: disableKeepAlives,
	}
	client := zms.NewClient(zmsURL, transport)
	return client, nil
}

func teardown() error {
	Global = nil
	log.Info("e2e teardown successfully")
	return nil
}
