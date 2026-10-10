/*
Copyright 2026.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CustomConfigSpec defines the desired state of CustomConfig. All fields
// except namespace map one-to-one onto a regular ConfigMap.
type CustomConfigSpec struct {
	// namespace names an existing CustomNamespace; the real ConfigMap is
	// created in the real namespace of that name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="namespace is immutable"
	Namespace string `json:"namespace"`

	// data carries the ConfigMap's string data.
	// +optional
	Data map[string]string `json:"data,omitempty"`

	// binaryData carries the ConfigMap's binary data.
	// +optional
	BinaryData map[string][]byte `json:"binaryData,omitempty"`

	// immutable mirrors the ConfigMap's immutable flag.
	// +optional
	Immutable *bool `json:"immutable,omitempty"`
}

// CustomConfigStatus defines the observed state of CustomConfig.
type CustomConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the CustomConfig resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// CustomConfig is the Schema for the customconfigs API
type CustomConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CustomConfig
	// +required
	Spec CustomConfigSpec `json:"spec"`

	// status defines the observed state of CustomConfig
	// +optional
	Status CustomConfigStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// CustomConfigList contains a list of CustomConfig
type CustomConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CustomConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &CustomConfig{}, &CustomConfigList{})
		return nil
	})
}
