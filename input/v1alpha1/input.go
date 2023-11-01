// Package v1beta1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=fn.crossplane.servicebinding.io
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// Decorator can be used to provide input to this Function.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Decorator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Config Config `json:"config"`
}

// Config specifies the configuration for the decorator
type Config struct {
	// specifies whether the decorator should assume all claims to specify spec.writeConnectionSecretToRef
	// if true, the decorator will always require the claim to specify spec.writeConnectionSecretToRef
	// if false, the decorator will create a binding secret if the claim does not specify
	// spec.writeConnectionSecretToRef or if spec.writeConnectionSecretToRef refers to a different namespace
	RequireWriteConnectionSecretToRef bool `json:"requireWriteConnectionSecretToRef"`

	// specifies the name of the provider config to use when creating the binding secret
	ProviderConfigRef *ProviderConfigRef `json:"providerConfigRef"`
}

// ProviderConfigRef specifies the provider config to use when creating the binding secret
type ProviderConfigRef struct {
	// specifies the name of the provider config to use when creating the binding secret
	Name string `json:"name"`
}
