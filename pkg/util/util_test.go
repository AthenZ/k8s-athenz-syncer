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
)

func newUtil() *Util {
	return NewUtil("admin.domain", []string{"kube-system", "kube-public", "kube-test"})
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