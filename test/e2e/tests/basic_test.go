package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/yahoo/athenz/clients/go/zms"
	"github.com/yahoo/k8s-athenz-syncer/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	domain         = zms.DomainName("k8s.omega.stage.kube-test")
	domainStr      = "k8s.omega.stage.kube-test"
	trustdomain    = zms.DomainName("prod-eng.omega.acceptancetest.trust-domain")
	trustdomainStr = "prod-eng.omega.acceptancetest.trust-domain"
	namespace      = "prod--eng-omega-acceptancetest-test--domain"
	nsdomain       = "prod-eng.omega.acceptancetest.test-domain"
	roleName       = zms.EntityName("syncer-e2e")
	roleNameStr    = zms.ResourceName(domainStr + ":role." + "syncer-e2e")
	roleResource   = zms.ResourceName("syncer-e2e")
	role           = zms.Role{
		Name: roleResource,
	}
	trustroleName     = zms.EntityName("test-trustrole")
	trustroleResource = zms.ResourceName("test-trustrole")
	trustRole         = zms.Role{
		Name:  trustroleResource,
		Trust: trustdomain,
	}
)

func TestBasicRoleUpdate(t *testing.T) {
	f := framework.Global
	err := f.ZMSClient.PutRole(domain, roleName, "", &role)
	if err != nil {
		t.Errorf("Unable to add role")
	}
	time.Sleep(90 * time.Second)
	cr, exists, err := f.CRClient.GetCRByName(domainStr)
	if err != nil {
		t.Error("Error while retrieving cr")
	}
	if !exists {
		t.Error("Did not find cr")
	}
	signedDomain := cr.Spec
	domainData := *signedDomain.Domain
	roles := domainData.Roles
	check := false
	for _, v := range roles {
		if v.Name == roleNameStr {
			check = true
		}
	}
	if !check {
		t.Error("Did not find the added role for the test")
	}
	fmt.Println("Test 1 finished")
}

func TestTrustDomain(t *testing.T) {
	f := framework.Global
	err := f.ZMSClient.PutRole(domain, trustroleName, "", &trustRole)
	if err != nil {
		t.Error("Unable to add trust role")
	}
	time.Sleep(90 * time.Second)
	cr, exists, err := f.CRClient.GetCRByName(domainStr)
	if !exists {
		t.Error("Did not find cr")
	}
	if err != nil {
		t.Error("Error while retrieving cr")
	}
	signedDomain := cr.Spec
	domainData := *signedDomain.Domain
	roles := domainData.Roles
	check := false
	for _, v := range roles {
		if v.Trust != "" && v.Trust == trustdomain {
			check = true
		}
	}
	if !check {
		t.Error("Did not find the added trust domain role in trust domain")
	}

	cr, exists, err = f.CRClient.GetCRByName(trustdomainStr)
	if err != nil {
		t.Error("Error fetching trust domain cr")
	}
	if !exists {
		t.Error("Did not find trust domain cr")
	}
	fmt.Println("Test 2 finished")
}
func TestNamespace(t *testing.T) {
	f := framework.Global
	_, err := f.K8sClient.CoreV1().Namespaces().Create(&v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})
	if err != nil {
		t.Error("Unable to create new namespace")
	}
	time.Sleep(90 * time.Second)
	_, exists, err := f.CRClient.GetCRByName(nsdomain)
	if err != nil {
		t.Error("Error while retrieving cr")
	}
	if !exists {
		t.Error("Did not fetch new cr for the namespace added")
	}
	fmt.Println("Test 3 finished")
}
