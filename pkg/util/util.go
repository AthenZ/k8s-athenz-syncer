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
)

// Util - struct with 2 fields adminDomain and list of system namespaces
type Util struct {
	adminDomain      string
	systemNamespaces []string
}

// NewUtil - create new Util object
func NewUtil(adminDomain string, systemNamespaces []string) *Util {
	return &Util{
		adminDomain:      adminDomain,
		systemNamespaces: systemNamespaces,
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
