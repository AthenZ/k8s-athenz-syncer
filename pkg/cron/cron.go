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
package cron

import (
	"context"
	"fmt"
	"time"

	"github.com/AthenZ/athenz/clients/go/zms"
	"github.com/AthenZ/k8s-athenz-syncer/pkg/cr"
	"github.com/AthenZ/k8s-athenz-syncer/pkg/log"
	"github.com/AthenZ/k8s-athenz-syncer/pkg/util"
	"github.com/cenkalti/backoff"
	corev1 "k8s.io/api/core/v1"
	apiError "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	trustDomainIndexKey = "trustDomain"
)

// AthenzContactTimeConfigMap for recording the latesttime that the Update Cron contacted Athenz
type AthenzContactTimeConfigMap struct {
	Namespace,
	Name,
	Key string
}

// Cron type for cron updates
type Cron struct {
	k8sClient     kubernetes.Interface
	checkInterval time.Duration
	syncInterval  time.Duration
	etag          string
	zmsClient     *zms.ZMSClient
	nsInformer    cache.SharedIndexInformer
	queue         workqueue.RateLimitingInterface
	util          *util.Util
	cr            *cr.CRUtil
	contactTimeCm *AthenzContactTimeConfigMap
}

// NewCron - creates new cron object
func NewCron(k8sClient kubernetes.Interface, checkInterval time.Duration, syncInterval time.Duration, etag string, zmsClient *zms.ZMSClient, informer cache.SharedIndexInformer, queue workqueue.RateLimitingInterface, util *util.Util, cr *cr.CRUtil, cm *AthenzContactTimeConfigMap) *Cron {
	return &Cron{
		k8sClient:     k8sClient,
		checkInterval: checkInterval,
		syncInterval:  syncInterval,
		etag:          etag,
		zmsClient:     zmsClient,
		nsInformer:    informer,
		queue:         queue,
		util:          util,
		cr:            cr,
		contactTimeCm: cm,
	}
}

// SetEtag - set initial etag in cron field
func (c *Cron) SetEtag(timestamp string) {
	c.etag = timestamp
}

// getExponentialBackoff - set parameters for exponential retries
func (c *Cron) getExponentialBackoff() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 2 * time.Second
	b.Multiplier = 2
	b.MaxElapsedTime = c.checkInterval / 2
	return b
}

// notifyOnErr - log error when the update cron retry fails
func notifyOnErr(err error, backoffDelay time.Duration) {
	log.Errorf("Failed to call zms to update syncer Cron: %s. Retrying in %s", err.Error(), backoffDelay)
}

// RequestCall - ZMS call for update crons
func (c *Cron) requestCall() error {
	master := false
	domains, etag, err := c.zmsClient.GetSignedDomains("", "true", "", &master,&master, c.etag)
	if err != nil {
		return fmt.Errorf("Error getting latest updated domains from ZMS API. Error: %v", err)
	}
	if err == nil && domains != nil && len(domains.Domains) > 0 {
		for _, domain := range domains.Domains {
			domainName := string(domain.Domain.Name)
			valid := c.ValidateDomain(domainName)
			if valid {
				c.queue.AddRateLimited(domainName)
			}
		}
	}
	if etag != "" {
		c.etag = etag
		c.UpdateAthenzContactTime(etag)
	}
	return nil
}

// UpdateCron - Run starts the main controller loop running sync at every poll interval
func (c *Cron) UpdateCron(stopCh <-chan struct{}) {
	for {
		log.Infoln("Athenz Update Cron Sleeping for", c.checkInterval)
		select {
		case <-stopCh:
			log.Infoln("Update Cron is stopped.")
			return
		case <-time.After(c.checkInterval):
			log.Infoln("Update Cron start to process updated Athenz Domains")
			backoff.RetryNotify(c.requestCall, c.getExponentialBackoff(), notifyOnErr)
		}
	}
}

// FullResync - add all namespaces to the queue for full resync
func (c *Cron) FullResync(stopCh <-chan struct{}) {
	for {
		log.Infoln("Full Resync Cron Sleeping for ", c.syncInterval)
		select {
		case <-stopCh:
			log.Infoln("Resync Cron is stopped.")
			return
		case <-time.After(c.syncInterval):
			log.Infoln("Full Resync Cron start to add all namespaces to work queue")
			// handle namespaces
			nslist := c.nsInformer.GetStore().List()
			for _, ns := range nslist {
				namespace, ok := ns.(*corev1.Namespace)
				if !ok {
					log.Error("Error occurred when casting namespace into string")
					continue
				}
				domainName := c.util.NamespaceToDomain(namespace.ObjectMeta.Name)
				c.queue.AddRateLimited(domainName)
			}
			// handle admin domain and system namespaces
			c.AddAdminSystemDomains()
			// handle trust domains
			// ListIndexFuncValues returns the list of keys of a particular index
			// it returns all the trust domains even if they're not used anymore
			trustdomains := c.cr.CrIndexInformer.GetIndexer().ListIndexFuncValues(trustDomainIndexKey)
			for _, domain := range trustdomains {
				// if the trust domain exist in informer store then we add to the queue
				_, exist, err := c.cr.CrIndexInformer.GetStore().GetByKey(domain)
				if err != nil {
					log.Errorf("Error occurred when checking trust domains in informder store. %v", err)
					continue
				}
				if exist {
					c.queue.AddRateLimited(domain)
				}
			}
		}
	}
}

// AddAdminSystemDomains - add admin domain and all the system domains to the queue
func (c *Cron) AddAdminSystemDomains() {
	adminDomain := c.util.GetAdminDomain()
	if adminDomain != "" {
		c.queue.AddRateLimited(adminDomain)
	}
	for _, domain := range c.util.GetSystemNSDomains() {
		if domain != "" {
			c.queue.AddRateLimited(domain)
		}
	}
}

// ValidateDomain - validate if the domain is whether a namespace, admin domain, system domain or trust domain
func (c *Cron) ValidateDomain(domain string) bool {
	namespace := c.util.DomainToNamespace(domain)
	_, exists, _ := c.nsInformer.GetIndexer().GetByKey(namespace)
	if exists || c.util.IsAdminDomain(domain) || c.cr.IsTrustDomain(domain) {
		return true
	}
	return false
}

// UpdateAthenzContactTime - update the latest athenz contact timestamp in config map
func (c *Cron) UpdateAthenzContactTime(etag string) {
	configmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.contactTimeCm.Name,
		},
		Data: map[string]string{c.contactTimeCm.Key: etag},
	}
	configMap, err := c.k8sClient.CoreV1().ConfigMaps(c.contactTimeCm.Namespace).Get(context.TODO(), c.contactTimeCm.Name, metav1.GetOptions{})
	if err != nil && apiError.IsNotFound(err) {
		configMap, err = c.k8sClient.CoreV1().ConfigMaps(c.contactTimeCm.Namespace).Create(context.TODO(), configmap, metav1.CreateOptions{})
		if err != nil {
			log.Errorf("Error occurred when creating new config map. Error: %v", err)
		}
	} else if err != nil {
		log.Errorf("Error occurred during GET config map. Error: %v", err)
		return
	} else if configMap != nil {
		_, err := c.k8sClient.CoreV1().ConfigMaps(c.contactTimeCm.Namespace).Update(context.TODO(), configmap, metav1.UpdateOptions{})
		if err != nil {
			log.Errorf("Unable to update latest timestamp in Config Map. Error: %v", err)
		}
	}
}
