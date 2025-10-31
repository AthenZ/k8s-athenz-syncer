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
	"fmt"
	"os"
	"strings"

	"github.com/AthenZ/athenz/clients/go/zms"
)

// Util - struct with 2 fields adminDomain and list of system namespaces
type Util struct {
	adminDomain       string
	systemNamespaces  []string
	excludeNamespaces map[string]bool
	excludeMSDRules   bool
}

// NewUtil - create new Util object
func NewUtil(adminDomain string, systemNamespaces []string, excludeNamespaces []string, excludeMSDRules bool) *Util {
	excludedNamespaceMap := make(map[string]bool)
	for _, ns := range excludeNamespaces {
		excludedNamespaceMap[ns] = true
	}

	return &Util{
		adminDomain:       adminDomain,
		systemNamespaces:  systemNamespaces,
		excludeNamespaces: excludedNamespaceMap,
		excludeMSDRules:   excludeMSDRules,
	}
}

// DomainToNamespace will convert an athenz domain to a kubernetes namespace. Dots are converted to dashes
// and dashes are converted to double dashes.
// ex: k8s.athenz-istio-auth -> k8s-athenz--istio--auth
func (u *Util) DomainToNamespace(domain string) (namespace string) {
	if u.adminDomain != "" && strings.HasPrefix(domain, u.adminDomain) {
		return strings.TrimPrefix(domain, u.adminDomain+".")
	}
	dubdash := strings.Replace(domain, "-", "--", -1)
	return strings.Replace(dubdash, ".", "-", -1)
}

// NamespaceToDomain will convert the kubernetes namespace to an athenz domain. Dashes are converted to dots and
// double dashes are converted to single dashes.
// ex: k8s-athenz--istio--auth -> k8s.athenz-istio-auth
func (u *Util) NamespaceToDomain(ns string) (domain string) {
	if u.isSystemNamespace(ns) {
		return fmt.Sprintf("%s.%s", u.adminDomain, ns)
	}
	dotted := strings.Replace(ns, "-", ".", -1)
	return strings.Replace(dotted, "..", "-", -1)
}

// isSystemNamespace - check if the current namespace is a system namespace
func (u *Util) isSystemNamespace(ns string) bool {
	for _, v := range u.systemNamespaces {
		if ns == v {
			return true
		}
	}
	return false
}

// IsSystemDomain - check if the current domain is a system domain
func (u *Util) IsSystemDomain(domain string) bool {
	for _, ns := range u.systemNamespaces {
		sysDomain := u.NamespaceToDomain(ns)
		if domain == sysDomain {
			return true
		}
	}
	return false
}

// IsNamespaceExcluded - check if the current namespace is in the skip namespaces list
func (u *Util) IsNamespaceExcluded(ns string) bool {
	if _, ok := u.excludeNamespaces[ns]; ok {
		return true
	}
	return false
}

// IsAdminDomain - check if domain is admin domain
func (u *Util) IsAdminDomain(domain string) bool {
	if domain == u.adminDomain {
		return true
	}
	return false
}

// GetAdminDomain - getter func for admin domain
func (u *Util) GetAdminDomain() string {
	return u.adminDomain
}

// GetSystemNSDomains - getter func for system ns
func (u *Util) GetSystemNSDomains() []string {
	domains := []string{}
	for _, ns := range u.systemNamespaces {
		domain := u.NamespaceToDomain(ns)
		domains = append(domains, domain)
	}
	return domains
}

// HomeDir - check evironment and return home directory
// Shared Functions with main.go and e2e test
func HomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func filterMSDRules(domain *zms.DomainData) *zms.DomainData {
	domainName := string(domain.Name)
	var filteredRoles []*zms.Role
	var filteredPolicies []*zms.Policy
	// Filter out roles where the names starts with "domainName:role.acl." which are MSD roles
	// Filter out roles that start with "domainName:role.msd-read-role-"
	for _, role := range domain.Roles {
		roleName := string(role.Name)
		aclPrefix := domainName + ":role.acl."
		msdReadPrefix := domainName + ":role.msd-read-role-"
		if strings.HasPrefix(roleName, aclPrefix) ||
			strings.HasPrefix(roleName, msdReadPrefix) {
			continue
		}
		filteredRoles = append(filteredRoles, role)
	}
	domain.Roles = filteredRoles

	// Filter out policies start with "domainName:policy.acl." which are MSD policies
	// Filter out policies start with "domainName:policy.msd-read-policy-"
	for _, policy := range domain.Policies.Contents.Policies {
		policyName := string(policy.Name)
		aclPrefix := domainName + ":policy.acl."
		msdReadPrefix := domainName + ":policy.msd-read-policy-"
		if strings.HasPrefix(policyName, aclPrefix) ||
			strings.HasPrefix(policyName, msdReadPrefix) {
			continue
		}
		filteredPolicies = append(filteredPolicies, policy)
	}
	domain.Policies.Contents.Policies = filteredPolicies

	return domain
}

func (u *Util) FilterMSDRules(domainData *zms.DomainData) *zms.DomainData {
	if !u.excludeMSDRules {
		return domainData
	}
	return filterMSDRules(domainData)
}
