// +build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/yahoo/athenz/clients/go/zms"
	"github.com/yahoo/k8s-athenz-syncer/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestBasicRoleUpdate(t *testing.T) {
	f := framework.Global
	domain := zms.DomainName(f.RoleDomain)
	roleName := zms.EntityName(f.RoleName)
	roleResource := zms.ResourceName(f.RoleName)
	role := zms.Role{
		Name: roleResource,
	}
	err := f.ZMSClient.PutRole(domain, roleName, "", &role)
	if err != nil {
		t.Errorf("Unable to add role")
	}
	err = wait.PollImmediate(time.Second*30, time.Minute*3, func() (bool, error) {
		cr, exists, err := f.CRClient.GetCRByName(f.RoleDomain)
		if err != nil {
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
		t.Error("Failed to find added role")
	}
}

func TestTrustDomain(t *testing.T) {
	f := framework.Global
	domain := zms.DomainName(f.RoleDomain)
	trustdomain := zms.DomainName(f.TrustDomain)
	trustroleName := zms.EntityName(f.TrustRole)
	trustroleResource := zms.ResourceName(f.TrustRole)
	trustRole := zms.Role{
		Name:  trustroleResource,
		Trust: trustdomain,
	}
	err := f.ZMSClient.PutRole(domain, trustroleName, "", &trustRole)
	if err != nil {
		t.Error("Unable to add trust role")
	}
	err = wait.PollImmediate(time.Second*30, time.Minute*3, func() (bool, error) {
		cr, exists, err := f.CRClient.GetCRByName(f.RoleDomain)
		if err != nil {
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
		t.Error("Did not find target trust domain resources")
	}
}

func TestNamespace(t *testing.T) {
	f := framework.Global
	namespace := f.MyUtil.DomainToNamespace(f.NamespaceDomain)
	_, err := f.K8sClient.CoreV1().Namespaces().Create(&v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})
	if err != nil {
		t.Error("Unable to create new namespace")
	}
	err = wait.PollImmediate(time.Second*30, time.Minute*3, func() (bool, error) {
		_, exists, err := f.CRClient.GetCRByName(f.NamespaceDomain)
		if err != nil {
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
		t.Error("Did not find the athenzdomains resource for the added namespace")
	}
}
