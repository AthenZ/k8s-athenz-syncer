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
	"regexp"
	"strings"

	"github.com/yahoo/athenz/clients/go/zms"
	v1 "github.com/yahoo/k8s-athenz-syncer/pkg/apis/athenz/v1"
	"k8s.io/client-go/tools/cache"
)

var roleReplacer = strings.NewReplacer("*", ".*", "?", ".", "^", "\\^", "$", "\\$", ".", "\\.", "|", "\\|", "[", "\\[", "+", "\\+", "\\", "\\\\", "(", "\\(", ")", "\\)", "{", "\\{")

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

// ProcessTrustDomain - in the delegated trust domain, under policy look for action assume_role which has current role as a resource,
// return role's member list
func ProcessTrustDomain(informer *cache.SharedIndexInformer, trust zms.DomainName, roleName string) ([]*zms.RoleMember, error) {
	var res []*zms.RoleMember
	// handle case which crIndexInformer is not initialized at the beginning, return directly.
	// if c.crIndexInformer == nil {
	// 	return res, nil
	// }
	trustDomain := string(trust)
	// initialize a clientset to get information of this trust athenz domain
	// storage := c.crIndexInformer.GetStore()
	crContent, exists, _ := (*informer).GetStore().GetByKey(trustDomain)
	if !exists {
		return res, fmt.Errorf("Error when finding trustDomain %s for this role name %s in the cache: Domain cr is not found in the cache store", trustDomain, roleName)
	}
	// cast it to AthenzDomain object
	obj, ok := crContent.(*v1.AthenzDomain)
	if !ok {
		return res, fmt.Errorf("Error occurred when casting trust domain interface to athen domain object")
	}

	for _, policy := range obj.Spec.SignedDomain.Domain.Policies.Contents.Policies {
		if policy == nil || len(policy.Assertions) == 0 {
			// if policy is empty, or policy doesn't have any assertions, continue with next policy
			continue
		}
		for _, assertion := range policy.Assertions {
			// check if policy contains action "assume_role", and resource matches with delegated role name
			if assertion.Action == "assume_role" {
				// form correct role name
				matched, err := regexp.MatchString("^"+roleReplacer.Replace(assertion.Resource)+"$", roleName)
				if err != nil {
					// if regexp matching fails with error, it should not procceed with current assertion
					continue
				}
				if matched {
					delegatedRole := assertion.Role
					// check if above policy's corresponding role is delegated role or not
					for _, role := range obj.Spec.SignedDomain.Domain.Roles {
						if string(role.Name) == delegatedRole {
							if role.Trust != "" {
								// return empty array since athenz zms library does not recursively check delegated domain
								// it only checks one level above. Refer to: https://github.com/yahoo/athenz/blob/master/servers/zms/src/main/java/com/yahoo/athenz/zms/DBService.java#L1972
								return res, nil
							}
							for _, member := range role.RoleMembers {
								res = append(res, member)
							}
							return res, nil
						}
					}
				}
			}
		}
	}
	return res, nil
}
