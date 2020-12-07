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
package controller

import (
	"errors"
	"fmt"
	"time"

	"github.com/ardielle/ardielle-go/rdl"
	"github.com/yahoo/k8s-athenz-syncer/pkg/cr"
	"github.com/yahoo/k8s-athenz-syncer/pkg/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/yahoo/athenz/clients/go/zms"
	athenzClientset "github.com/yahoo/k8s-athenz-syncer/pkg/client/clientset/versioned"
	athenzInformer "github.com/yahoo/k8s-athenz-syncer/pkg/client/informers/externalversions/athenz/v1"
	"github.com/yahoo/k8s-athenz-syncer/pkg/cron"
	"github.com/yahoo/k8s-athenz-syncer/pkg/ratelimiter"
	"github.com/yahoo/k8s-athenz-syncer/pkg/util"
)

const (
	workerQueueRetry    = 3
	trustDomainIndexKey = "trustDomain"
)

// Controller struct defines how a controller should encapsulate
// logging, client connectivity, informing (list and watching)
// queueing, and handling of resource changes
type Controller struct {
	clientset       kubernetes.Interface
	queue           workqueue.RateLimitingInterface
	nsIndexInformer cache.SharedIndexInformer
	zmsClient       *zms.ZMSClient
	cron            *cron.Cron
	util            *util.Util
	cr              *cr.CRUtil
}

// NewController returns a Controller with logger, clientset, queue and informer generated
func NewController(k8sClient kubernetes.Interface, versiondClient athenzClientset.Interface, zmsClient *zms.ZMSClient, updateCron time.Duration, resyncCron time.Duration, delayInterval time.Duration, util *util.Util, cm *cron.AthenzContactTimeConfigMap) *Controller {
	nsListWatcher := cache.NewListWatchFromClient(k8sClient.CoreV1().RESTClient(), "namespaces", corev1.NamespaceAll, fields.Everything())
	nsIndexInformer := cache.NewSharedIndexInformer(nsListWatcher, &corev1.Namespace{}, time.Hour, cache.Indexers{})
	rateLimiter := ratelimiter.NewRateLimiter(delayInterval)
	queue := workqueue.NewRateLimitingQueue(rateLimiter)
	c := &Controller{
		clientset:       k8sClient,
		queue:           queue,
		nsIndexInformer: nsIndexInformer,
		zmsClient:       zmsClient,
		util:            util,
	}
	c.addNSInformerHandlers(nsIndexInformer)
	// initialize cr informer
	crIndexInformer := athenzInformer.NewAthenzDomainInformer(versiondClient, 0, cache.Indexers{})
	crIndexInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			domain := c.crinformerhandler(cache.MetaNamespaceKeyFunc, obj)
			log.Infof("AthenzDomain CR Add Event Created. Domain: %s", domain)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			domain := c.crinformerhandler(cache.MetaNamespaceKeyFunc, newObj)
			log.Infof("AthenzDomain CR Update Event Created. Domain: %s", domain)
		},
		DeleteFunc: func(obj interface{}) {
			domain := c.crinformerhandler(cache.DeletionHandlingMetaNamespaceKeyFunc, obj)
			log.Infof("AthenzDomain CR Delete Event Created. Domain: %s", domain)
		},
	})
	crIndexInformer.AddIndexers(cache.Indexers{
		trustDomainIndexKey: cr.TrustDomainIndexFunc,
	})
	c.cr = cr.NewCRUtil(versiondClient, crIndexInformer)
	c.cron = cron.NewCron(k8sClient, updateCron, resyncCron, "", zmsClient, nsIndexInformer, queue, util, c.cr, cm)
	return c
}

// addNSInformerHandlers - add handlers for nsIndexInformer
func (c *Controller) addNSInformerHandlers(nsIndexInformer cache.SharedIndexInformer) {
	nsIndexInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key := c.nsinformerhandler(cache.MetaNamespaceKeyFunc, obj)
			log.Infof("Add namespace: %s", key)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key := c.nsinformerhandler(cache.MetaNamespaceKeyFunc, newObj)
			log.Infof("Update cluster namespace: %s", key)
		},
		DeleteFunc: func(obj interface{}) {
			key := c.nsinformerhandler(cache.DeletionHandlingMetaNamespaceKeyFunc, obj)
			log.Infof("Delete namespace: %s", key)
		},
	})
}

// nsinformerhandler - event handler for nsIndexInformer
func (c *Controller) nsinformerhandler(fn cache.KeyFunc, obj interface{}) string {
	key, err := fn(obj)
	if err != nil {
		log.Errorf("Error returned from Key Func in nsInformerHandler. Error: %v", err)
		return ""
	}
	domain := c.util.NamespaceToDomain(key)
	c.queue.AddRateLimited(domain)
	return key
}

// crinformerhandler - helper function for crIndexInformer handler
func (c *Controller) crinformerhandler(fn cache.KeyFunc, obj interface{}) string {
	key, err := fn(obj)
	if err != nil {
		log.Errorf("Error returned from Key Func in crInformerHandler. Error: %v", err)
		return ""
	}
	c.queue.AddRateLimited(key)
	return key
}

// Run is the main path of execution for the controller loop
func (c *Controller) Run(stopCh <-chan struct{}) {
	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()
	// ignore new items in the queue but when all goroutines
	// have completed existing items then shutdown
	defer c.queue.ShutDown()

	log.Info("Controller.Run: initiating")

	// run the nsinformer and crinformer to start listing and watching resources
	go c.nsIndexInformer.Run(stopCh)
	go c.cr.CrIndexInformer.Run(stopCh)

	// do the initial synchronization (one time) to populate resources
	if !cache.WaitForCacheSync(stopCh, c.nsIndexInformer.HasSynced, c.cr.CrIndexInformer.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Error syncing cache"))
		return
	}
	log.Info("Controller.Run: cache sync complete")

	timestamp := c.cr.GetLatestTimestamp()
	c.cron.SetEtag(timestamp)
	go c.cron.UpdateCron(stopCh)
	go c.cron.FullResync(stopCh)

	// add all admin domain and system namespaces to the queue initially
	c.cron.AddAdminSystemDomains()

	// run the runWorker method every second with a stop channel
	wait.Until(c.runWorker, time.Second, stopCh)
}

// runWorker executes the loop to process new items added to the queue
func (c *Controller) runWorker() {
	// invoke processNextItem to fetch and consume the next change
	// to a watched or listed resource
	for c.processNextItem() {
	}
}

// processNextItem retrieves each queued item and takes the
// necessary handler action based off of if the item was
// created or deleted
func (c *Controller) processNextItem() bool {
	log.Debug("Controller.processNextItem: start")

	// fetch the next item (blocking) from the queue to process or
	// if a shutdown is requested then return out of this to stop
	// processing
	key, quit := c.queue.Get()

	// stop the worker loop from running as this indicates we
	// have sent a shutdown message that the queue has indicated
	// from the Get method
	if quit {
		return false
	}

	defer c.queue.Done(key)

	// assert the string out of the key (format `domainName`)
	domainName, ok := key.(string)
	if !ok {
		log.Errorf("string cast failed. Key object: %v", key)
		return true
	}
	log.Info("Processing key: ", domainName)

	// process item that is popped off
	err := c.sync(domainName)
	// retry when there is a 429 or there is something wrong with create/update CR
	if err != nil {
		if c.queue.NumRequeues(domainName) < workerQueueRetry {
			c.queue.AddRateLimited(domainName)
			log.Infof("Error processing AthenzDomain CR (name: %s) in Athenz database: %v. Retrying...", domainName, err)
		} else {
			c.queue.Forget(key)
			log.Infof("Error processing AthenzDomain CR (name: %s) in Athenz database. End of Retry.", domainName)
		}
	}

	// keep the worker loop running by returning true
	return true
}

// sync - process queue item
func (c *Controller) sync(domain string) error {
	// if this domain is not a valid domain(a domain that we want to sync) then we attempt to remove it
	valid := c.cron.ValidateDomain(domain)
	if !valid {
		log.Errorf("Domain %s is an invalid domain (not part of namespace, admin domain, system domain or trust domain)", domain)
		return c.cr.RemoveAthenzDomain(domain)
	}
	result, exist, err := c.zmsGetSignedDomains(domain)
	if err != nil {
		log.Errorf("Error while making ZMS get signed domainName (%s): %v", domain, err)
		rdl, ok := err.(rdl.ResourceError)
		if !ok {
			return errors.New("Error occurred when converting error types")
		}
		// if return 404 error, remove AthenzDomains CR
		if rdl.Code == 404 {
			return c.cr.RemoveAthenzDomain(domain)
		}
		obj, exists, err := c.cr.GetCRByName(domain)
		if err != nil {
			return err
		}
		if exists {
			obj.Status.Message = err.Error()
			c.cr.UpdateErrorStatus(obj)
		}
		return err
	}
	if !exist {
		log.Errorf("Did not find DomainName: %s in ZMS.", domain)
		return c.cr.RemoveAthenzDomain(domain)
	}
	zmsDomainName := zms.DomainName(domain)
	for _, domainData := range result.Domains {
		if domainData.Domain.Name == zmsDomainName {
			_, err := c.cr.CreateUpdateAthenzDomain(domain, domainData)
			if err != nil {
				return fmt.Errorf("Error occurred when creating AthenzDomain custom resources. Error: %v", err)
			}
			log.Infof("Successfully created/updated new AthenzDomains CR: %v", zmsDomainName)
			// parse domain data and add trust domains to the queue
			for _, role := range domainData.Domain.Roles {
				if role != nil && string(role.Trust) != "" && role.Trust != zmsDomainName {
					_, exists, err := c.cr.CrIndexInformer.GetStore().GetByKey(string(role.Trust))
					if err != nil {
						log.Errorf("Error checking trust domain in cache. Error: %v", err)
						continue
					}
					if !exists {
						// Here the logic is to check if current domain is a namespace existing in the cluster, if so, add the trust domain to the queue
						// otherwise, we should skip processing trust domain as athenz zms only checks one level above for delegated domains.
						_, nsExists, err := c.nsIndexInformer.GetStore().GetByKey(c.util.DomainToNamespace(domain))
						if err != nil {
							log.Errorf("Error checking domain's corresponding namespace in namespace cache store. Error: %v", err)
							continue
						}
						if nsExists || c.util.IsAdminDomain(domain) {
							c.queue.AddRateLimited(string(role.Trust))
						}
					}
				}
			}
		}
	}
	return nil
}

// zmsGetSignedDomains - make http request to zms API to fetch domain data
func (c *Controller) zmsGetSignedDomains(domain string) (*zms.SignedDomains, bool, error) {
	d := zms.DomainName(domain)
	signedDomain, _, err := c.zmsClient.GetSignedDomains(d, "", "", "")
	if err != nil {
		return nil, false, err
	}
	// Currently for GetSignedDomains API call, it returns {"domains":[]} when domain (d) passed in does not exist in Athenz
	if len(signedDomain.Domains) == 0 {
		log.Error("SignedDomain call returned an empty list")
		return nil, false, nil
	}
	return signedDomain, true, nil
}
