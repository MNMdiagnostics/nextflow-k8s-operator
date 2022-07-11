/*
Copyright 2022.

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Nextflow-specific configuration
type NextflowLaunchNextflow struct {
	Image         string   `json:"image,omitempty"`
	Version       string   `json:"version,omitempty"`
	Command       []string `json:"command,omitempty"`
	ScmSecretName string   `json:"scmSecretName,omitempty"`
}

// NextflowLaunchSpec defines the desired state of NextflowLaunch
type NextflowLaunchSpec struct {
	Pipeline string                 `json:"pipeline,omitempty"`
	Nextflow NextflowLaunchNextflow `json:"nextflow,omitempty"`
	K8s      map[string]string      `json:"k8s,omitempty"`
	Pod      []map[string]string    `json:"pod,omitempty"`
	Params   map[string]string      `json:"params,omitempty"`
	Env      map[string]string      `json:"env,omitempty"`
}

// NextflowLaunchStatus defines the observed state of NextflowLaunch
type NextflowLaunchStatus struct {
	Stage     string                  `json:"stage,omitempty"`
	MainPod   *corev1.ObjectReference `json:"mainpod,omitempty"`
	ConfigMap *corev1.ObjectReference `json:"configmap,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NextflowLaunch is the Schema for the nextflowlaunches API
type NextflowLaunch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NextflowLaunchSpec   `json:"spec,omitempty"`
	Status NextflowLaunchStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NextflowLaunchList contains a list of NextflowLaunch
type NextflowLaunchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NextflowLaunch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NextflowLaunch{}, &NextflowLaunchList{})
}
