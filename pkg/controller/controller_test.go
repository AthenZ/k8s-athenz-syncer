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
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/ardielle/ardielle-go/rdl"
	"github.com/google/go-cmp/cmp"
	"github.com/yahoo/athenz/clients/go/zms"
	athenz_domain "github.com/yahoo/k8s-athenz-syncer/pkg/apis/athenz/v1"
	"github.com/yahoo/k8s-athenz-syncer/pkg/client/clientset/versioned/fake"
	"github.com/yahoo/k8s-athenz-syncer/pkg/cron"
	"github.com/yahoo/k8s-athenz-syncer/pkg/util"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

const (
	domainName = "home.domain"
	username   = "user.name"
)

func newController() *Controller {
	athenzclientset := fake.NewSimpleClientset()
	clientset := k8sfake.NewSimpleClientset()
	zmsclient := zms.NewClient("https://zms.athenz.com", &http.Transport{})
	util := util.NewUtil("admin.domain", []string{"kube-system", "kube-public", "kube-test"})
	cm := &cron.AthenzContactTimeConfigMap{
		Namespace: "kube-yahoo",
		Name:      "athenzcall-config",
		Key:       "latest_contact",
	}
	newCtl := NewController(clientset, athenzclientset, &zmsclient, time.Minute, time.Hour, 250*time.Millisecond, util, cm)
	return newCtl
}

func getFakeDomain() zms.SignedDomain {
	t := true
	f := false
	allow := zms.ALLOW
	timestamp, err := rdl.TimestampParse("2019-06-21T19:28:09.305Z")
	if err != nil {
		panic(err)
	}

	return zms.SignedDomain{
		Domain: &zms.DomainData{
			Enabled:      &t,
			AuditEnabled: &f,
			Modified:     timestamp,
			Name:         zms.DomainName(domainName),
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
							Active:     &f,
							Approved:   &f,
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
			Groups: []*zms.Group {},
		},
		KeyId:     "colo-env-1.1",
		Signature: "signature",
	}
}

func comparePointers() cmp.Option {
	return cmp.FilterPath(func(p cmp.Path) bool {
		t := p.Last().Type()
		return t != nil && t.Kind() == reflect.Ptr
	}, cmp.FilterValues(func(x, y interface{}) bool {
		vx := reflect.ValueOf(x)
		vy := reflect.ValueOf(y)
		return vx.IsValid() && vy.IsValid() && vx.Kind() == reflect.Ptr && vy.Kind() == reflect.Ptr
	}, cmp.Comparer(func(x, y interface{}) bool {
		vx := reflect.ValueOf(x)
		vy := reflect.ValueOf(y)
		if vx.IsNil() && vy.IsNil() {
			return true
		}
		return vx.IsValid() && vy.IsValid() && vx.Pointer() != vy.Pointer() && cmp.Equal(vx.Elem().Interface(), vy.Elem().Interface(), comparePointers())
	})))
}

func TestDeepCopy(t *testing.T) {
	obj := &athenz_domain.AthenzDomain{
		Spec: athenz_domain.AthenzDomainSpec{
			SignedDomain: getFakeDomain(),
		},
	}

	newObj := obj.DeepCopy()
	if !cmp.Equal(obj.Spec, newObj.Spec, comparePointers()) {
		t.Error("Error old object and new deep copy are shallow copies, expected deep copy")
	}

	newObj = obj
	if cmp.Equal(obj.Spec, newObj.Spec, comparePointers()) {
		t.Error("Error old object and new deep copy are shallow copies, expected deep copy")
	}

	newObj = obj.DeepCopy()
	newObj.Spec.Domain.Roles = obj.Spec.Domain.Roles
	if cmp.Equal(obj.Spec, newObj.Spec, comparePointers()) {
		t.Error("Error old object and new deep copy are shallow copies, expected deep copy")
	}
}

//TestZmsGetSignedDomains - test zms API call to get signed domains
func TestZmsGetSignedDomains(t *testing.T) {
	d := getFakeDomain()
	signedDomain := zms.SignedDomains{
		Domains: []*zms.SignedDomain{&d},
	}

	js, _ := json.Marshal(&signedDomain)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(js)
	})
	httpClient, teardown := testingHTTPClient(h)
	defer teardown()
	c := newController()
	c.zmsClient.Transport = httpClient.Transport
	res, _, err := c.zmsGetSignedDomains(domainName)
	if err != nil {
		t.Error("Failed to get signed domain", err)
	}

	expectedDomain := getFakeDomain()
	if !reflect.DeepEqual(res.Domains[0], &expectedDomain) {
		t.Error("Failed to get the correct result")
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
