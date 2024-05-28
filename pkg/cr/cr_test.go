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
package cr

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/AthenZ/athenz/clients/go/zms"
	athenz_domain "github.com/AthenZ/k8s-athenz-syncer/pkg/apis/athenz/v1"
	"github.com/AthenZ/k8s-athenz-syncer/pkg/client/clientset/versioned/fake"
	athenzInformer "github.com/AthenZ/k8s-athenz-syncer/pkg/client/informers/externalversions/athenz/v1"
	"github.com/AthenZ/k8s-athenz-syncer/pkg/log"
	"github.com/ardielle/ardielle-go/rdl"
	apiError "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	domainName = "home.domain"
	username   = "user.name"
	keyName    = "home-domain"
)

func getFakeDomain() zms.SignedDomain {
	allow := zms.ALLOW
	timestamp, err := rdl.TimestampParse("2019-06-21T19:28:09.305Z")
	if err != nil {
		panic(err)
	}

	return zms.SignedDomain{
		Domain: &zms.DomainData{
			Modified: timestamp,
			Name:     zms.DomainName(domainName),
			Policies: &zms.SignedPolicies{
				Contents: &zms.DomainPolicies{
					Domain: zms.DomainName(domainName),
					Policies: []*zms.Policy{
						{
							Assertions: []*zms.Assertion{
								{
									Role:     domainName + ":role.admin",
									Resource: domainName + ".test:*",
									Action:   "*",
									Effect:   &allow,
								},
							},
							Modified: &timestamp,
							Name:     zms.ResourceName(domainName + ":policy.admin"),
						},
					},
				},
				KeyId:     "col-env-1.1",
				Signature: "signature-policy",
			},
			Roles: []*zms.Role{
				{
					Members:  []zms.MemberName{zms.MemberName(username)},
					Modified: &timestamp,
					Name:     zms.ResourceName(domainName + ":role.admin"),
					RoleMembers: []*zms.RoleMember{
						{
							MemberName: zms.MemberName(username),
						},
					},
				},
				{
					Trust:    "parent.domain",
					Modified: &timestamp,
					Name:     zms.ResourceName(domainName + ":role.trust"),
				},
			},
			Services: []*zms.ServiceIdentity{},
			Entities: []*zms.Entity{},
		},
		KeyId:     "colo-env-1.1",
		Signature: "signature",
	}
}

func newCRResource() *CRUtil {
	athenzclientset := fake.NewSimpleClientset()
	informer := athenzInformer.NewAthenzDomainInformer(athenzclientset, 0, cache.Indexers{
		"trustDomain": TrustDomainIndexFunc,
	})
	return NewCRUtil(athenzclientset, informer)
}

// TestCreateUpdateCR - test create/update new AthenzDomain CR
func TestCreateUpdateCR(t *testing.T) {
	signedDomain := getFakeDomain()
	c := newCRResource()

	// test Create CR functionality
	cr, err := c.CreateUpdateAthenzDomain(context.TODO(), domainName, &signedDomain)
	if err != nil {
		t.Error("Failed to create CR successfully", err)
	}
	res, err := c.athenzClientset.AthenzDomains().Get(context.TODO(), domainName, v1.GetOptions{})
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(res.Spec.SignedDomain, getFakeDomain()) {
		t.Error("Signed domain are not equal")
	}
	cr.ObjectMeta = metav1.ObjectMeta{
		Name: "home.domain",
	}
	c.CrIndexInformer.GetStore().Add(cr)

	// test Update existing CR functionality
	signedDomain.KeyId = "300"
	updateRes, err := c.CreateUpdateAthenzDomain(context.TODO(), domainName, &signedDomain)
	if err != nil {
		t.Error("Failed to update CR successfully", err)
	}
	if updateRes.Spec.SignedDomain.KeyId != "300" {
		t.Error("Error")
	}
}

// TestRemoveAthenzDomain - test remove new AthenzDomain CR
func TestRemoveAthenzDomain(t *testing.T) {
	log.InitLogger("/tmp/log/test.log", "info")
	signedDomain := getFakeDomain()
	c := newCRResource()

	// create cr
	cr, err := c.CreateUpdateAthenzDomain(context.TODO(), domainName, &signedDomain)
	if err != nil {
		t.Error("Failed to create CR successfully", err)
	}
	cr.ObjectMeta = metav1.ObjectMeta{
		Name: "home.domain",
	}
	c.CrIndexInformer.GetStore().Add(cr)

	err = c.RemoveAthenzDomain(context.TODO(), domainName)
	if err != nil {
		t.Error(err)
	}
	res, err := c.athenzClientset.AthenzDomains().Get(context.TODO(), domainName, v1.GetOptions{})
	if !apiError.IsNotFound(err) {
		t.Error(err)
	}
	if res != nil {
		t.Error(res)
	}
}

func TestGetLatestTimestamp(t *testing.T) {
	times := [3]string{"2019-05-27T21:53:45Z", "2019-05-29T21:53:45Z", "2019-05-29T21:50:45Z"}
	c := newCRResource()
	for i, v := range times {
		domainName := fmt.Sprintf("home.czhuang.test.%d", i)
		time, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t.Errorf("Error parsing time formats. Error: %v", err)
		}
		domain := zms.DomainData{
			Name: zms.DomainName(domainName),
			Modified: rdl.Timestamp{
				Time: time,
			},
		}
		signedDomain := zms.SignedDomain{
			Domain: &domain,
		}
		cr, err := c.CreateUpdateAthenzDomain(context.TODO(), domainName, &signedDomain)
		if err != nil {
			t.Errorf("Error occurred during create/update of CR. Error: %v", err)
		}
		c.CrIndexInformer.GetStore().Add(cr)
	}
	str := c.GetLatestTimestamp()
	if str != "2019-05-29T21:53:45Z" {
		t.Error("Did not get the latest timestamp")
	}
}

// TestIsTrustDomain - test whether input domain is a trust domain or not
func TestIsTrustDomain(t *testing.T) {
	c := newCRResource()
	result := c.IsTrustDomain(domainName)
	if result {
		t.Error("home.domain is not a trust domain")
	}
	childDomain := &athenz_domain.AthenzDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name: "child.domain",
		},
		Spec: athenz_domain.AthenzDomainSpec{
			SignedDomain: getFakeDomain(),
		},
	}
	trustDomain := &athenz_domain.AthenzDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name: "parent.domain",
		},
		Spec: athenz_domain.AthenzDomainSpec{
			SignedDomain: getFakeDomain(),
		},
	}
	c.CrIndexInformer.GetIndexer().Add(childDomain)
	c.CrIndexInformer.GetIndexer().Add(trustDomain)
	result2 := c.IsTrustDomain("parent.domain")
	if !result2 {
		t.Error("parent.domain is a trust domain")
	}
}

// TestTrustDomainIndexFunc - test indexer func
func TestTrustDomainIndexFunc(t *testing.T) {
	childDomain := &athenz_domain.AthenzDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name: "child.domain",
		},
		Spec: athenz_domain.AthenzDomainSpec{
			SignedDomain: getFakeDomain(),
		},
	}
	arr, err := TrustDomainIndexFunc(childDomain)
	if err != nil {
		t.Error(err)
	}
	if arr[0] != "parent.domain" {
		t.Error("failed to parse trust domains")
	}
}
