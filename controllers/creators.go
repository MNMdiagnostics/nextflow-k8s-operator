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

package controllers

import (
	"bytes"
	"errors"
	"strconv"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	batchv1alpha1 "mnmdiagnostics/nextflow-k8s-operator/api/v1alpha1"
)

const (
	defaultMountPath       = "/workspace"
	defaultNextflowImage   = "nextflow/nextflow"
	defaultNextflowVersion = "22.06.0-edge"
	configPath             = "/tmp/nextflow.config"

	statusRunning   = "Running"
	statusSucceeded = "Succeeded"
	statusFailed    = "Failed"
)

// Construct a Pod object for Nextflow
func makeNextflowPod(nfLaunch batchv1alpha1.NextflowLaunch, configMapName string) corev1.Pod {

	spec := nfLaunch.Spec

	// the main NF pod
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nfLaunch.Name + "-" + generateHash(8),
			Namespace: nfLaunch.Namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image:   spec.Nextflow.Image + ":" + spec.Nextflow.Version,
				Command: spec.Nextflow.Command,
				Name:    nfLaunch.Name + "-" + generateHash(8),
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "nextflow-config",
						MountPath: configPath,
						SubPath:   "nextflow.config",
					},
					{
						Name:      "nextflow-volume",
						MountPath: spec.K8s["storageMountPath"],
					},
				},
			}},
			Volumes: []corev1.Volume{
				{
					Name: "nextflow-config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: configMapName,
							},
						},
					},
				},
				{
					Name: "nextflow-volume",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: spec.K8s["storageClaimName"],
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	// optionally attach a secret volume with scm data in it
	if nfLaunch.Spec.Nextflow.ScmSecretName != "" {
		pod.Spec.Containers[0].VolumeMounts = append(
			pod.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "nextflow-scm",
				MountPath: "/.nextflow/scm",
				SubPath:   "scm",
			},
		)
		pod.Spec.Volumes = append(
			pod.Spec.Volumes,
			corev1.Volume{
				Name: "nextflow-scm",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: spec.Nextflow.ScmSecretName,
					},
				},
			},
		)
	}

	return pod
}

// Construct a Nextflow config file as a ConfigMap
func makeNextflowConfig(nfLaunch batchv1alpha1.NextflowLaunch) corev1.ConfigMap {

	configTemplate, _ := template.New("config").Parse(`
        process {
           executor = 'k8s'
           pod = [
           {{- range $opt := .Pod -}}
           [
           {{- range $key, $value := $opt -}}
           {{ js $key }}: '{{ js $value }}',
           {{- end -}}
           ],
           {{- end -}}
           ]
        }
        k8s {
           {{- range $par, $value := .K8s }}
           {{ $par }} = '{{ js $value }}'
           {{- end }}
        }
        params {
           {{- range $par, $value := .Params }}
           {{ $par }} = '{{ js $value }}'
           {{- end }}
        }
        env {
           {{- range $par, $value := .Env }}
           {{ $par }} = '{{ js $value }}'
           {{- end }}
        }`)

	type Options struct {
		K8s    map[string]string
		Params map[string]string
		Env    map[string]string
		Pod    []map[string]string
	}
	values := Options{
		K8s:    nfLaunch.Spec.K8s,
		Params: nfLaunch.Spec.Params,
		Env:    nfLaunch.Spec.Env,
		Pod:    nfLaunch.Spec.Pod,
	}
	var config bytes.Buffer
	configTemplate.Execute(&config, values)

	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nfLaunch.Name + "-nextflow-config-" + generateHash(8),
			Namespace: nfLaunch.Namespace,
		},
		Data: map[string]string{
			"nextflow.config": config.String(),
		},
	}
}

// Validate launch definition, return an error or nil
func validateLaunch(nfLaunch batchv1alpha1.NextflowLaunch) (batchv1alpha1.NextflowLaunch, error) {

	spec := nfLaunch.Spec

	// validation
	if spec.Pipeline == "" {
		return nfLaunch, errors.New("spec.Pipeline is required")
	}
	if keyIsEmpty(spec.K8s, "storageClaimName") {
		return nfLaunch, errors.New("spec.k8s.storageClaimName is required")
	}

	// defaults for the essential settings
	if keyIsEmpty(spec.K8s, "storageMountPath") {
		spec.K8s["storageMountPath"] = defaultMountPath
	}
	if keyIsEmpty(spec.K8s, "launchDir") {
		spec.K8s["launchDir"] = spec.K8s["storageMountPath"] + "/" + nfLaunch.Name
	}
	if keyIsEmpty(spec.K8s, "workDir") {
		spec.K8s["workDir"] = spec.K8s["launchDir"] + "/work"
	}
	if spec.Nextflow.Image == "" {
		spec.Nextflow.Image = defaultNextflowImage
	}
	if spec.Nextflow.Version == "" {
		spec.Nextflow.Version = defaultNextflowVersion
	}
	profileArg := ""
	if spec.Profile != "" {
		profileArg = "-profile " + strconv.QuoteToASCII(spec.Profile)
	}
	if len(spec.Nextflow.Command) == 0 {
		spec.Nextflow.Command = []string{
			"nextflow", "run",
			"-c", configPath,
			"-w", strconv.QuoteToASCII(spec.K8s["workDir"]),
			profileArg,
			strconv.QuoteToASCII(spec.Pipeline),
		}
	}
	nfLaunch.Spec = spec
	return nfLaunch, nil
}
