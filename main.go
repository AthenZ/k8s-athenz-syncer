/*
Copyright 2019, Oath Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/yahoo/k8s-athenz-syncer/pkg/controller"
	"github.com/yahoo/k8s-athenz-syncer/pkg/cron"
	"github.com/yahoo/k8s-athenz-syncer/pkg/crypto"
	"github.com/yahoo/k8s-athenz-syncer/pkg/identity"
	"github.com/yahoo/k8s-athenz-syncer/pkg/util"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/yahoo/athenz/clients/go/zms"
	athenzClientset "github.com/yahoo/k8s-athenz-syncer/pkg/client/clientset/versioned"
	"github.com/yahoo/k8s-athenz-syncer/pkg/log"
	r "github.com/yahoo/k8s-athenz-syncer/pkg/reloader"
)

// getClients retrieve the Kubernetes cluster client and Athenz client
func getClients(inClusterConfig *bool) (kubernetes.Interface, *athenzClientset.Clientset, error) {
	var kubeconfig *string
	if home := util.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()
	if *inClusterConfig {
		emptystr := ""
		kubeconfig = &emptystr
	}
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Panicln(err.Error())
	}

	// generate the client based off of the config
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to create k8s client from config. Error: %v", err)
	}

	versiondClient, err := athenzClientset.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to create versiond client from config. Error: %v", err)
	}

	log.Info("Successfully constructed k8s client")
	return client, versiondClient, nil
}

// createZMSClient - create client to zms to make zms calls
func createZMSClient(reloader *r.CertReloader, zmsURL string, disableKeepAlives bool) (*zms.ZMSClient, error) {
	config := &tls.Config{}
	config.GetClientCertificate = func(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
		return reloader.GetLatestCertificate(), nil
	}
	transport := &http.Transport{
		TLSClientConfig:   config,
		DisableKeepAlives: disableKeepAlives,
	}
	client := zms.NewClient(zmsURL, transport)
	return &client, nil
}

// main code path
func main() {
	// command line arguments for athenz initial setup
	key := flag.String("key", "/var/run/athenz/service.key.pem", "Athenz private key file")
	cert := flag.String("cert", "/var/run/athenz/service.cert.pem", "Athenz certificate file")
	zmsURL := flag.String("zms-url", "", "Athenz ZMS API URL")
	updateCron := flag.String("update-cron", "1m0s", "Update cron sleep time")
	athenzContactTimeCmNs := flag.String("athenz-contact-time-cm-ns", "kube-yahoo", "Namespace of ConfigMap to record the latest time that the Update Cron contacted Athenz")
	athenzContactTimeCmName := flag.String("athenz-contact-time-cm-name", "athenzcall-config", "Name of ConfigMap to record the latest time that the Update Cron contacted Athenz")
	athenzContactTimeCmKey := flag.String("athenz-contact-time-cm-key", "latest_contact", "Key of ConfigMap to record the latest time that the Update Cron contacted Athenz")
	resyncCron := flag.String("resync-cron", "1h0m0s", "Cron full resync sleep time")
	queueDelayInterval := flag.String("queue-delay-interval", "250ms", "Delay interval time for workqueue")
	adminDomain := flag.String("admin-domain", "", "admin domain")
	systemNamespaces := flag.String("system-namespaces", "", "list of cluster system namespaces")
	disableKeepAlives := flag.Bool("disable-keep-alives", true, "Disable keep alive for zms client")
	logLoc := flag.String("log-location", "/var/log/k8s-athenz-syncer/k8s-athenz-syncer.log", "log location")
	logMode := flag.String("log-mode", "info", "logger mode")
	identityKeyDir := flag.String("identity-key", "/var/run/keys/identity", "directory containing private keys for service identity")
	useNToken := flag.Bool("use-ntoken", false, "use nToken for zms authentication")
	serviceName := flag.String("service-name", "k8s-athenz-syncer", "service name")
	domainName := flag.String("service-domain", "", "athenz domain that contains k8s-athenz-syncer")
	secretName := flag.String("secret-name", "k8s-athenz-syncer", "secret name that contains private key")
	header := flag.String("auth-header", "", "Authentication header field")
	nTokenExpireTime := flag.String("ntoken-expiry", "1h0m0s", "Custom nToken expiration duration")

	// create new log
	log.InitLogger(*logLoc, *logMode)
	// get the Kubernetes and Athenz client for connectivity
	inClusterConfig := flag.Bool("inClusterConfig", true, "Set to true to use in cluster config.")
	k8sClient, versiondClient, err := getClients(inClusterConfig)
	if err != nil {
		log.Panicf("Error occurred when creating clients. Error: %v", err)
	}

	stopCh := make(chan struct{})
	var zmsClient *zms.ZMSClient
	if *useNToken {
		client := zms.NewClient(*zmsURL, nil)
		zmsClient = &client

		// custom nToken expiration duration
		nTokenPeriod, err := time.ParseDuration(*nTokenExpireTime)
		if err != nil {
			log.Panicf("NToken expiry duration input is invalid. Error: %v", err)
		}

		privateKeySource := crypto.NewPrivateKeySource(*identityKeyDir, *secretName)
		// create tokenProvider
		_, err = identity.NewTokenProvider(identity.Config{
			Client:             zmsClient,
			Header:             *header,
			Domain:             *domainName,
			Service:            *serviceName,
			PrivateKeyProvider: privateKeySource.SigningKey,
			TokenExpiry:        nTokenPeriod,
		}, stopCh)
		if err != nil {
			log.Panicf("Could not create new Token Provider: %v", err)
		}
		log.Info("Sucessfully created ZMS Client with nToken authn")
	} else {
		// setup key cert reloader
		certReloader, err := r.NewCertReloader(r.ReloadConfig{
			KeyFile:  *key,
			CertFile: *cert,
		}, stopCh)
		if err != nil {
			log.Panicf("Error occurred when creating new reloader. Error: %v", err)
		}
		// use key and cert to create zmsClient for API calls
		zmsClient, err = createZMSClient(certReloader, *zmsURL, *disableKeepAlives)
		if err != nil {
			log.Panicf("Error occurred when creating zms client. Error: %v", err)
		}
		log.Info("Sucessfully created ZMS Client with certs authn")
	}

	// process system-namespaces input string and create new Util object
	systemNSList := strings.Split(*systemNamespaces, ",")
	processList := []string{}
	for _, item := range systemNSList {
		if item != "" {
			processList = append(processList, item)
		}
	}
	util := util.NewUtil(*adminDomain, processList)

	// construct the Controller object which has all of the necessary components to
	// handle logging, connections, informing (listing and watching), the queue,
	// and the handler
	updatePeriod, err := time.ParseDuration(*updateCron)
	if err != nil {
		log.Panicf("Update Cron interval input is invalid. Error: %v", err)
	}
	resyncPeriod, err := time.ParseDuration(*resyncCron)
	if err != nil {
		log.Panicf("Full Resync Cron interval input is invalid. Error: %v", err)
	}
	delayInterval, err := time.ParseDuration(*queueDelayInterval)
	if err != nil {
		log.Panicf("Queue delay input is invalid. Error: %v", err)
	}

	cm := &cron.AthenzContactTimeConfigMap{
		Namespace: *athenzContactTimeCmNs,
		Name:      *athenzContactTimeCmName,
		Key:       *athenzContactTimeCmKey,
	}

	controller := controller.NewController(k8sClient, versiondClient, zmsClient, updatePeriod, resyncPeriod, delayInterval, util, cm)

	// use a channel to synchronize the finalization for a graceful shutdown
	defer close(stopCh)

	// run the controller loop to process items
	go controller.Run(stopCh)

	// use a channel to handle OS signals to terminate and gracefully shut
	// down processing
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm
}
