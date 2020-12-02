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
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ardielle/ardielle-go/rdl"
	"github.com/yahoo/athenz/clients/go/zms"
	"github.com/yahoo/k8s-athenz-syncer/pkg/client/clientset/versioned/fake"
	athenzInformer "github.com/yahoo/k8s-athenz-syncer/pkg/client/informers/externalversions/athenz/v1"
	"github.com/yahoo/k8s-athenz-syncer/pkg/cr"
	"github.com/yahoo/k8s-athenz-syncer/pkg/log"
	"github.com/yahoo/k8s-athenz-syncer/pkg/ratelimiter"
	"github.com/yahoo/k8s-athenz-syncer/pkg/util"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

func newCron() *Cron {
	etag := "2019-07-01T21:53:45Z"
	zmsClient := zms.NewClient("https://zms.athenz.com", &http.Transport{})
	clientset := k8sfake.NewSimpleClientset()
	rateLimiter := ratelimiter.NewRateLimiter(250 * time.Millisecond)
	queue := workqueue.NewRateLimitingQueue(rateLimiter)
	util := util.NewUtil("test.domain", []string{"kube-system"})
	athenzclientset := fake.NewSimpleClientset()
	informer := athenzInformer.NewAthenzDomainInformer(athenzclientset, 0, cache.Indexers{
		"trustDomain": cr.TrustDomainIndexFunc,
	})
	nsListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "namespaces", corev1.NamespaceAll, fields.Everything())
	nsIndexInformer := cache.NewSharedIndexInformer(nsListWatcher, &corev1.Namespace{}, time.Hour, cache.Indexers{})
	nsIndexInformer.GetStore().Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "home-test",
	}})
	cr := cr.NewCRUtil(athenzclientset, informer)
	cm := &AthenzContactTimeConfigMap{
		Namespace: "kube-yahoo",
		Name:      "athenzcall-config",
		Key:       "latest_contact",
	}
	return NewCron(clientset, 20*time.Second, time.Minute, etag, &zmsClient, nsIndexInformer, queue, util, cr, cm)
}

func TestRequestCall(t *testing.T) {
	log.InitLogger("/tmp/log/test.log", "info")
	newtime, err := time.Parse(time.RFC3339, "2019-07-05T21:53:45Z")
	if err != nil {
		t.Errorf("Failed to parse fake time. Error: %v", err)
	}
	domain := zms.DomainData{
		Name: zms.DomainName("home.test"),
		Modified: rdl.Timestamp{
			Time: newtime,
		},
	}
	d := zms.SignedDomain{
		Domain: &domain,
	}
	arr := make([]*zms.SignedDomain, 1)
	arr[0] = &d
	signedDomain := zms.SignedDomains{
		Domains: arr,
	}
	js, err := json.Marshal(&signedDomain)
	if err != nil {
		t.Errorf("Failed to marshal signedDomain into json. Error: %v", err)
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Etag", "2019-07-05T21:53:45Z")
		w.Write(js)
	})
	httpClient, teardown := testingHTTPClient(h)
	defer teardown()
	c := newCron()
	c.zmsClient.Transport = httpClient.Transport
	err = c.requestCall()
	if err != nil {
		t.Error("Failed to get signed domain", err)
	}
	if c.queue.Len() != 1 {
		t.Error("Expected queue length is 1. Failed to add to queue.")
	}
	if c.etag != "2019-07-05T21:53:45Z" {
		t.Errorf("Failed to update to new etag after update cron runs. Current etag: %s", c.etag)
	}
}

// testingHTTPClient - helper function to mock http requests
func testingHTTPClient(handler http.Handler) (*http.Client, func()) {
	s := httptest.NewTLSServer(handler)

	cli := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
				return net.Dial(network, s.Listener.Addr().String())
			},
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	return cli, s.Close
}

// TestAddAdminSystemDomains - add admin domains
func TestAddAdminSystemDomains(t *testing.T) {
	c := newCron()
	c.AddAdminSystemDomains()
	time.Sleep(time.Second)
	if c.queue.Len() != 2 {
		t.Error(c.queue)
		t.Error("Wrong queue length")
	}
}

// TestValidateDomain - test for valid domains
func TestValidateDomain(t *testing.T) {
	c := newCron()
	log.InitLogger("/tmp/log/test.log", "info")
	res1 := c.ValidateDomain("test.domain")
	if !res1 {
		t.Error("test.domain is a valid domain")
	}
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}
	c.nsInformer.GetIndexer().Add(ns)
	res2 := c.ValidateDomain("test.domain.kube-system")
	if !res2 {
		t.Error("test.domain.kube-system is a valid domain")
	}
}

// UpdateAthenzContactTime - test for update athenz contact time in configmap
func TestUpdateAthenzContactTime(t *testing.T) {
	c := newCron()
	log.InitLogger("/tmp/log/test.log", "info")
	c.UpdateAthenzContactTime("2019-01-01T01:01:01.111Z")
	configMap, err := c.k8sClient.CoreV1().ConfigMaps(c.contactTimeCm.Namespace).Get(context.TODO(), c.contactTimeCm.Name, metav1.GetOptions{})
	if err != nil {
		t.Error(err)
	}
	if configMap == nil {
		t.Error("New config map created should not be nil")
	}
	c.UpdateAthenzContactTime("2020-02-02T01:01:01.111Z")
	configMap, err = c.k8sClient.CoreV1().ConfigMaps(c.contactTimeCm.Namespace).Get(context.TODO(), c.contactTimeCm.Name, metav1.GetOptions{})
	if configMap.Data[c.contactTimeCm.Key] != "2020-02-02T01:01:01.111Z" {
		t.Error("Failed to update the latest timestamp")
	}
}
