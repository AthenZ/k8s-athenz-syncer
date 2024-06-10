//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/AthenZ/athenz/clients/go/zms"
	"github.com/AthenZ/k8s-athenz-syncer/pkg/log"
	"github.com/AthenZ/k8s-athenz-syncer/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// TestBasicRoleUpdate - adds a new role in Athenz and tests if syncer updates the corresponding AthenzDomains resource
func TestBasicRoleUpdate(t *testing.T) {
	f := framework.Global
	// pre-check to see if the target athenzdomains resource exists before modifying
	_, exists, err := f.CRClient.GetCRByName(f.RoleDomain)
	if err != nil {
		log.Errorf("test 1 Pre-Check Error finding cr: %v", err)
		t.Error("test 1 Pre-Check Error while finding athenzdomain resource from store")
	}
	if !exists {
		t.Error("Pre-Check Error: Did not find target domain cr")
	}
	// adding role to Athenz
	domain := zms.DomainName(f.RoleDomain)
	roleName := zms.EntityName(f.RoleName)
	roleResource := zms.ResourceName(f.RoleName)
	role := zms.Role{
		Name: roleResource,
	}
	err = f.ZMSClient.PutRole(domain, roleName, "", &role)
	if err != nil {
		log.Errorf("test 1 error adding regular role: %v", err)
		t.Errorf("Unable to add role")
	}
	// checking for updates in cr
	err = wait.PollImmediate(time.Second*30, time.Minute*3, func() (bool, error) {
		cr, exists, err := f.CRClient.GetCRByName(f.RoleDomain)
		if err != nil {
			log.Errorf("test 1 error getting athenzdomains for test: %v", err)
			t.Log("Error while finding athenzdomain resource from store")
			return false, nil
		}
		if !exists {
			t.Log("Did not find cr")
			return false, nil
		}
		signedDomain := cr.Spec
		domainData := *signedDomain.Domain
		roles := domainData.Roles
		roleNameStr := zms.ResourceName(f.RoleDomain + ":role." + f.RoleName)
		check := false
		for _, v := range roles {
			if v.Name == roleNameStr {
				check = true
			}
		}
		if !check {
			t.Log("Did not find added role")
			return false, nil

		}
		return true, nil
	})
	if err != nil {
		log.Errorf("test 1 failed, error: %v", err)
		t.Error("Failed to find added role")
	}
}

// TestTrustDomain - adds a trust role in Athenz and tests if syncer updates the original AthenzDomains resource
// and if a new trust domain AthenzDomains is created
func TestTrustDomain(t *testing.T) {
	f := framework.Global
	// pre-check to see if the target domain exists and trust domain does not exist
	_, exists, err := f.CRClient.GetCRByName(f.RoleDomain)
	if err != nil {
		log.Errorf("test 2 Pre-Check error finding target cr: %v", err)
		t.Error("Pre-Check Error while finding athenzdomains resource in store")
	}
	if !exists {
		t.Error("Pre-Check Error: Did not find target domain cr")
	}
	_, exists, err = f.CRClient.GetCRByName(f.TrustDomain)
	if err != nil {
		log.Errorf("test 2 Pre-Check error finding trust cr: %v", err)
		t.Error("Pre-Check Error while finding trust domain resource from store")
	}
	if exists {
		t.Error("Pre-Check Error: trust domain cr already exists before test")
	}
	// adding trust role to Athenz
	domain := zms.DomainName(f.RoleDomain)
	trustdomain := zms.DomainName(f.TrustDomain)
	trustroleName := zms.EntityName(f.TrustRole)
	trustroleResource := zms.ResourceName(f.TrustRole)
	trustRole := zms.Role{
		Name:  trustroleResource,
		Trust: trustdomain,
	}
	err = f.ZMSClient.PutRole(domain, trustroleName, "", &trustRole)
	if err != nil {
		log.Errorf("test 2 unable to add trust role: %v", err)
		t.Error("Unable to add trust role")
	}
	// checking for updates in cr
	err = wait.PollImmediate(time.Second*30, time.Minute*3, func() (bool, error) {
		cr, exists, err := f.CRClient.GetCRByName(f.RoleDomain)
		if err != nil {
			log.Errorf("test 2 error finding target cr in test: %v", err)
			t.Log("Error while finding athenzdomains resource in store")
			return false, nil
		}
		if !exists {
			t.Log("Did not find domain cr")
			return false, nil
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
			t.Log("Did not find added trust role")
			return false, nil
		}

		cr, exists, err = f.CRClient.GetCRByName(f.TrustDomain)
		if err != nil {
			log.Errorf("test 2 error finding trust domain in test: %v", err)
			t.Log("Error while finding trust domain resource from store")
			return false, nil
		}
		if !exists {
			t.Log("Did not find trust domain cr")
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		log.Errorf("test 2 failed, error: %v", err)
		t.Error("Did not find target trust domain resources")
	}
}

// TestNamespace - adds a namespace in cluster and checks if syncer creates AthenzDomains resource for the new namespace
func TestNamespace(t *testing.T) {
	f := framework.Global
	// pre-check to see namespace domain does not exist before test
	_, exists, err := f.CRClient.GetCRByName(f.NamespaceDomain)
	if err != nil {
		log.Errorf("test 3 Pre-Check error finding target cr: %v", err)
		t.Error("Pre-Check Error while finding athenzdomains resource from store")
	}
	if exists {
		t.Error("Pre-Check Error: Namespace Domain cr already exists in store before test")
	}
	// adding namespace to cluster
	namespace := f.MyUtil.DomainToNamespace(f.NamespaceDomain)
	_, err = f.K8sClient.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		log.Errorf("test 3 unable to create namespace: %v", err)
		t.Error("Unable to create new namespace")
	}
	// checking new namespace domain cr
	err = wait.PollImmediate(time.Second*30, time.Minute*3, func() (bool, error) {
		_, exists, err := f.CRClient.GetCRByName(f.NamespaceDomain)
		if err != nil {
			log.Errorf("test 3 error finding target cr: %v", err)
			t.Log("Error while finding athenzdomains resource from store")
			return false, nil
		}
		if !exists {
			t.Log("Did not find cr")
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		log.Errorf("test 3 failed, error: %v", err)
		t.Error("Did not find the athenzdomains resource for the added namespace")
	}
}
