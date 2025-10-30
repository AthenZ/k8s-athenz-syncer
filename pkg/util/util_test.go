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
package util

import (
	"testing"

	"github.com/AthenZ/athenz/clients/go/zms"
)

func newUtil() *Util {
	return NewUtil("admin.domain", []string{"kube-system", "kube-public", "kube-test"}, []string{"acceptance-test"}, false)
}

// TestNamespaceConversion - test conversion function
func TestNamespaceConversion(t *testing.T) {
	u := newUtil()
	namespace := "hello-world-test"
	result := u.NamespaceToDomain(namespace)
	if result != "hello.world.test" {
		t.Errorf("Namespace converted wrong")
	}
	doubledash := "hello--world-test"
	result = u.NamespaceToDomain(doubledash)
	if result != "hello-world.test" {
		t.Errorf("Namespace with double dash converted wrong")
	}
	system := "kube-system"
	result = u.NamespaceToDomain(system)
	if result != "admin.domain.kube-system" {
		t.Errorf("System Namespaces converted wrong")
	}
}

// TestDomainConversion - test conversion function
func TestDomainConversion(t *testing.T) {
	u := newUtil()
	domain := "hello.world.test"
	result := u.DomainToNamespace(domain)
	if result != "hello-world-test" {
		t.Errorf("Domain converted wrong")
	}
	doubledash := "hello-world.test"
	result = u.DomainToNamespace(doubledash)
	if result != "hello--world-test" {
		t.Errorf("Namespace with double dash converted wrong")
	}
	system := "admin.domain.kube-system"
	result = u.DomainToNamespace(system)
	if result != "kube-system" {
		t.Errorf("System namespaces converted wrong")
	}
}

// TestIsSystemNamespace - test whether certain namespace is a system namespace
func TestIsSystemNamespace(t *testing.T) {
	u := newUtil()
	test1 := "hello-world"
	result1 := u.isSystemNamespace(test1)
	if result1 {
		t.Error("test1 failed. hello-world is not a system namespace!")
	}
	test2 := "kube-system"
	result2 := u.isSystemNamespace(test2)
	if !result2 {
		t.Error("test2 failed. kube-system is a system-namespace!")
	}
}

// TestIsAdminDomain - test whether certain domain is an admin domain
func TestIsAdminDomain(t *testing.T) {
	u := newUtil()
	test1 := "admin.domain"
	result1 := u.IsAdminDomain(test1)
	if !result1 {
		t.Error("test1 failed. admin.domain is an admin domain!")
	}
	test2 := "kube-system"
	result2 := u.IsAdminDomain(test2)
	if result2 {
		t.Error("test2 failed. kube-system is not an admin domain!")
	}
}

// TestGetAdminDomain - test GetAdminDomain returns the right admin domain
func TestGetAdminDomain(t *testing.T) {
	u := newUtil()
	result := u.GetAdminDomain()
	if result != "admin.domain" {
		t.Error("GetAdminDomain returned the wrong admin domain")
	}
}

// TestGetSystemNSDomains - test GetAdminDomain returns the right system domains
func TestGetSystemNSDomains(t *testing.T) {
	u := newUtil()
	ans := []string{"admin.domain.kube-system", "admin.domain.kube-public", "admin.domain.kube-test"}
	result := u.GetSystemNSDomains()
	for i := range result {
		if result[i] != ans[i] {
			t.Error("GetSystemNSDomains returned the wrong system domain " + result[i])
		}
	}
}

// TestIsNamespaceExcluded - test whether certain namespace is in the skip namespaces list
func TestIsNamespaceExcluded(t *testing.T) {
	u := newUtil()
	test1 := "acceptance-test"
	result1 := u.IsNamespaceExcluded(test1)
	if !result1 {
		t.Error("test1 failed. acceptance-test is in the skip namespaces list!")
	}
	test2 := "kube-system"
	result2 := u.IsNamespaceExcluded(test2)
	if result2 {
		t.Error("test2 failed. kube-system is not in the skip namespaces list!")
	}
}

// TestFilterMSDRules - table-driven tests for filtering MSD roles and policies
func TestFilterMSDRules(t *testing.T) {
	tests := []struct {
		name             string
		domainName       string
		roles            []*zms.Role
		policies         []*zms.Policy
		expectedRoles    []string
		expectedPolicies []string
		description      string
	}{
		{
			name:       "mixed MSD and regular roles and policies",
			domainName: "test.domain",
			roles: []*zms.Role{
				{Name: zms.ResourceName("test.domain:role.regular-role")},
				{Name: zms.ResourceName("test.domain:role.acl.msd-role")},
				{Name: zms.ResourceName("test.domain:role.another-regular")},
				{Name: zms.ResourceName("test.domain:role.acl.another-msd")},
				{Name: zms.ResourceName("test.domain:role.msd-read-role")},
				{Name: zms.ResourceName("test.domain:role.msd-read-role-suffix")},
			},
			policies: []*zms.Policy{
				{Name: zms.ResourceName("test.domain:policy.regular-policy")},
				{Name: zms.ResourceName("test.domain:policy.acl.msd-policy")},
				{Name: zms.ResourceName("test.domain:policy.another-regular")},
				{Name: zms.ResourceName("test.domain:policy.acl.another-msd")},
				{Name: zms.ResourceName("test.domain:policy.msd-read-policy-")},
				{Name: zms.ResourceName("test.domain:policy.msd-read-policy-suffix")},
			},
			expectedRoles: []string{
				"test.domain:role.regular-role",
				"test.domain:role.another-regular",
				"test.domain:role.msd-read-role",
			},
			expectedPolicies: []string{
				"test.domain:policy.regular-policy",
				"test.domain:policy.another-regular",
			},
			description: "should filter out MSD roles and policies while preserving regular ones",
		},
		{
			name:             "empty domain with no roles or policies",
			domainName:       "empty.domain",
			roles:            []*zms.Role{},
			policies:         []*zms.Policy{},
			expectedRoles:    []string{},
			expectedPolicies: []string{},
			description:      "should handle empty domain correctly",
		},
		{
			name:       "only MSD roles and policies",
			domainName: "msd.domain",
			roles: []*zms.Role{
				{Name: zms.ResourceName("msd.domain:role.acl.msd1")},
				{Name: zms.ResourceName("msd.domain:role.acl.msd2")},
				{Name: zms.ResourceName("msd.domain:role.msd-read-role-test")},
			},
			policies: []*zms.Policy{
				{Name: zms.ResourceName("msd.domain:policy.acl.msd1")},
				{Name: zms.ResourceName("msd.domain:policy.acl.msd2")},
				{Name: zms.ResourceName("msd.domain:policy.msd-read-policy-")},
				{Name: zms.ResourceName("msd.domain:policy.msd-read-policy-test")},
			},
			expectedRoles:    []string{},
			expectedPolicies: []string{},
			description:      "should filter out all items when everything is MSD",
		},
		{
			name:       "no MSD roles or policies",
			domainName: "regular.domain",
			roles: []*zms.Role{
				{Name: zms.ResourceName("regular.domain:role.admin")},
				{Name: zms.ResourceName("regular.domain:role.reader")},
			},
			policies: []*zms.Policy{
				{Name: zms.ResourceName("regular.domain:policy.admin-policy")},
				{Name: zms.ResourceName("regular.domain:policy.reader-policy")},
			},
			expectedRoles: []string{
				"regular.domain:role.admin",
				"regular.domain:role.reader",
			},
			expectedPolicies: []string{
				"regular.domain:policy.admin-policy",
				"regular.domain:policy.reader-policy",
			},
			description: "should preserve all items when none are MSD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domainData := &zms.DomainData{
				Name:  zms.DomainName(tt.domainName),
				Roles: tt.roles,
				Policies: &zms.SignedPolicies{
					Contents: &zms.DomainPolicies{
						Policies: tt.policies,
					},
				},
			}

			result := filterMSDRules(domainData)

			// Check roles
			if len(result.Roles) != len(tt.expectedRoles) {
				t.Errorf("%s: expected %d roles, got %d", tt.description, len(tt.expectedRoles), len(result.Roles))
			}

			actualRoles := make(map[string]bool)
			for _, role := range result.Roles {
				actualRoles[string(role.Name)] = true
			}

			for _, expectedRole := range tt.expectedRoles {
				if !actualRoles[expectedRole] {
					t.Errorf("%s: expected role '%s' not found in result", tt.description, expectedRole)
				}
			}

			// Verify no unexpected roles (MSD roles should be filtered)
			for _, role := range result.Roles {
				roleName := string(role.Name)
				found := false
				for _, expectedRole := range tt.expectedRoles {
					if roleName == expectedRole {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: unexpected role '%s' found in result", tt.description, roleName)
				}
			}

			// Check policies
			if len(result.Policies.Contents.Policies) != len(tt.expectedPolicies) {
				t.Errorf("%s: expected %d policies, got %d", tt.description, len(tt.expectedPolicies), len(result.Policies.Contents.Policies))
			}

			actualPolicies := make(map[string]bool)
			for _, policy := range result.Policies.Contents.Policies {
				actualPolicies[string(policy.Name)] = true
			}

			for _, expectedPolicy := range tt.expectedPolicies {
				if !actualPolicies[expectedPolicy] {
					t.Errorf("%s: expected policy '%s' not found in result", tt.description, expectedPolicy)
				}
			}

			// Verify no unexpected policies (MSD policies should be filtered)
			for _, policy := range result.Policies.Contents.Policies {
				policyName := string(policy.Name)
				found := false
				for _, expectedPolicy := range tt.expectedPolicies {
					if policyName == expectedPolicy {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: unexpected policy '%s' found in result", tt.description, policyName)
				}
			}
		})
	}
}

// TestUtilFilterMSDRules - table-driven tests for Util.FilterMSDRules method
func TestUtilFilterMSDRules(t *testing.T) {
	tests := []struct {
		name             string
		excludeMSDRules  bool
		expectedRoles    []string
		expectedPolicies []string
		description      string
	}{
		{
			name:            "with excludeMSDRules flag enabled",
			excludeMSDRules: true,
			expectedRoles: []string{
				"test.domain:role.regular",
				"test.domain:role.msd-read-role",
			},
			expectedPolicies: []string{
				"test.domain:policy.regular",
			},
			description: "should filter out MSD roles and policies when flag is true",
		},
		{
			name:            "with excludeMSDRules flag disabled",
			excludeMSDRules: false,
			expectedRoles: []string{
				"test.domain:role.regular",
				"test.domain:role.acl.msd",
				"test.domain:role.msd-read-role",
				"test.domain:role.msd-read-role-test",
			},
			expectedPolicies: []string{
				"test.domain:policy.regular",
				"test.domain:policy.acl.msd",
				"test.domain:policy.msd-read-policy-test",
			},
			description: "should preserve all roles and policies when flag is false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domainName := "test.domain"

			roles := []*zms.Role{
				{Name: zms.ResourceName(domainName + ":role.regular")},
				{Name: zms.ResourceName(domainName + ":role.acl.msd")},
				{Name: zms.ResourceName(domainName + ":role.msd-read-role")},
				{Name: zms.ResourceName(domainName + ":role.msd-read-role-test")},
			}

			policies := []*zms.Policy{
				{Name: zms.ResourceName(domainName + ":policy.regular")},
				{Name: zms.ResourceName(domainName + ":policy.acl.msd")},
				{Name: zms.ResourceName(domainName + ":policy.msd-read-policy-test")},
			}

			domainData := &zms.DomainData{
				Name:  zms.DomainName(domainName),
				Roles: roles,
				Policies: &zms.SignedPolicies{
					Contents: &zms.DomainPolicies{
						Policies: policies,
					},
				},
			}

			util := NewUtil("admin.domain", []string{"kube-system"}, []string{}, tt.excludeMSDRules)
			result := util.FilterMSDRules(domainData)

			// Check roles
			if len(result.Roles) != len(tt.expectedRoles) {
				t.Errorf("%s: expected %d roles, got %d", tt.description, len(tt.expectedRoles), len(result.Roles))
			}

			actualRoles := make(map[string]bool)
			for _, role := range result.Roles {
				actualRoles[string(role.Name)] = true
			}

			for _, expectedRole := range tt.expectedRoles {
				if !actualRoles[expectedRole] {
					t.Errorf("%s: expected role '%s' not found in result", tt.description, expectedRole)
				}
			}

			// Check policies
			if len(result.Policies.Contents.Policies) != len(tt.expectedPolicies) {
				t.Errorf("%s: expected %d policies, got %d", tt.description, len(tt.expectedPolicies), len(result.Policies.Contents.Policies))
			}

			actualPolicies := make(map[string]bool)
			for _, policy := range result.Policies.Contents.Policies {
				actualPolicies[string(policy.Name)] = true
			}

			for _, expectedPolicy := range tt.expectedPolicies {
				if !actualPolicies[expectedPolicy] {
					t.Errorf("%s: expected policy '%s' not found in result", tt.description, expectedPolicy)
				}
			}
		})
	}
}
