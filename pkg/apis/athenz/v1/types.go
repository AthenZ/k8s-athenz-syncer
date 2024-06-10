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
package v1

import (
	"github.com/mohae/deepcopy"
	"github.com/AthenZ/athenz/clients/go/zms"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AthenzDomain is a top-level type
type AthenzDomain struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Status AthenzDomainStatus `json:"status,omitempty"`

	// Athenz Domain Spec
	Spec AthenzDomainSpec `json:"spec,omitempty"`
}

// AthenzDomainSpec contains the SignedDomain object https://github.com/AthenZ/athenz/clients/go/zms
type AthenzDomainSpec struct {
	zms.SignedDomain `json:",inline"`
}

// DeepCopy copies the object and returns a clone
func (in *AthenzDomainSpec) DeepCopy() *AthenzDomainSpec {
	if in == nil {
		return nil
	}
	outRaw := deepcopy.Copy(in)
	out, ok := outRaw.(*AthenzDomainSpec)
	if !ok {
		return nil
	}
	return out
}

// AthenzDomainStatus stores status information about the current resource
type AthenzDomainStatus struct {
	Message string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AthenzDomainList is a list of AthenzDomain items
type AthenzDomainList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AthenzDomain `json:"items"`
}
