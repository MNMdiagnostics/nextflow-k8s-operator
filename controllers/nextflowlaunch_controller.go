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
	"context"
	goerrors "errors"
	"math/rand"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	batchv1alpha1 "mnmdiagnostics/nextflow-k8s-operator/api/v1alpha1"
)

// NextflowLaunchReconciler reconciles a NextflowLaunch object
type NextflowLaunchReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	defaultNextflowImage   = "nextflow/nextflow"
	defaultNextflowVersion = "22.06.0-edge"

	statusRunning   = "Running"
	statusSucceeded = "Succeeded"
	statusFailed    = "Failed"
)

//+kubebuilder:rbac:groups=batch.mnm.bio,resources=nextflowlaunches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch.mnm.bio,resources=nextflowlaunches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch.mnm.bio,resources=nextflowlaunches/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get

// Generate a hexadecimal hash of the specified length
func generateHash(n int) string {
	const pool = "0123456789abcdef"
	s := make([]byte, n)
	for i := range s {
		s[i] = pool[rand.Intn(len(pool))]
	}
	return string(s)
}

// Construct a Pod object for Nextflow
func makeNextflowPod(nfLaunch batchv1alpha1.NextflowLaunch, configMapName string) corev1.Pod {

	nextflowImage := nfLaunch.Spec.Nextflow.Image
	if nextflowImage == "" {
		nextflowImage = defaultNextflowImage
	}
	nextflowVersion := nfLaunch.Spec.Nextflow.Version
	if nextflowVersion == "" {
		nextflowVersion = defaultNextflowVersion
	}
	nextflowCommand := nfLaunch.Spec.Nextflow.Command
	if len(nextflowCommand) == 0 {
		nextflowCommand = []string{
			"nextflow", "run",
			"-c", "/tmp/nextflow.config",
			"-w", "/workspace", // FIXME: hard-coded path
			nfLaunch.Spec.Pipeline,
		}
	}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nfLaunch.Name + "-" + generateHash(8),
			Namespace: nfLaunch.Namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image:   nextflowImage + ":" + nextflowVersion,
				Command: nextflowCommand,
				Name:    nfLaunch.Name + "-" + generateHash(8),
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "nextflow-config",
						MountPath: "/tmp/nextflow.config",
						SubPath:   "nextflow.config",
					},
					{
						Name:      "nextflow-volume",
						MountPath: "/workspace", // FIXME: hard-coded path
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
							ClaimName: nfLaunch.Spec.K8s["storageClaimName"],
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
						SecretName: nfLaunch.Spec.Nextflow.ScmSecretName,
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
func validateLaunch(nfLaunch batchv1alpha1.NextflowLaunch) error {
	spec := nfLaunch.Spec

	value, hasKey := spec.K8s["storageClaimName"]
	if !hasKey || value == "" {
		return goerrors.New("spec.k8s.storageClaimName is required")
	}
	return nil
}

// Reconciler function for NextflowLaunch
func (r *NextflowLaunchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := log.FromContext(ctx)
	var nfLaunch batchv1alpha1.NextflowLaunch

	err := r.Get(ctx, req.NamespacedName, &nfLaunch)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Nextflow launch " + req.Name + " deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Error fetching Nextflow launch "+req.Name)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	err = validateLaunch(nfLaunch)
	if err != nil {
		log.Error(err, "Incorrect launch definition (yaml file)")
		return ctrl.Result{}, nil
	}

	stage := nfLaunch.Status.Stage

	if stage == statusRunning {
		// job is running, retrieve child pod to check status
		var pod corev1.Pod
		podName := types.NamespacedName{
			Namespace: nfLaunch.Status.MainPod.Namespace,
			Name:      nfLaunch.Status.MainPod.Name,
		}
		err = r.Get(ctx, podName, &pod)
		if err != nil {
			log.Error(err, "Error fetching pod")
			return ctrl.Result{}, err
		}
		status := pod.Status.Phase
		log.Info("Job running (" + string(status) + ")")

		if status == corev1.PodSucceeded {
			nfLaunch.Status.Stage = statusSucceeded
			r.Status().Update(ctx, &nfLaunch)

		} else if status == corev1.PodFailed {
			nfLaunch.Status.Stage = statusFailed
			r.Status().Update(ctx, &nfLaunch)

		} else {
			// come revisit later
			return ctrl.Result{RequeueAfter: 3e+9}, nil
		}

	} else if stage == statusSucceeded {
		// job has finished successfully
		log.Info("Job finished.")

	} else if stage == statusFailed {
		// job has failed
		log.Info("Job failed! Use `kubectl logs " + nfLaunch.Status.MainPod.Name +
			"` to diagnose")

	} else {
		// job is ready to run, create children
		configMap := makeNextflowConfig(nfLaunch)
		ctrl.SetControllerReference(&nfLaunch, &configMap, r.Scheme)
		err = r.Client.Create(ctx, &configMap)
		if err != nil {
			log.Error(err, "Error creating Nextflow config")
			return ctrl.Result{}, err
		}
		nfLaunch.Status.ConfigMap, _ = reference.GetReference(r.Scheme, &configMap)

		pod := makeNextflowPod(nfLaunch, configMap.Name)
		ctrl.SetControllerReference(&nfLaunch, &pod, r.Scheme)
		log.Info("Starting pod " + pod.Name)
		err = r.Client.Create(ctx, &pod)
		if err != nil {
			log.Error(err, "Error creating pod")
			return ctrl.Result{}, err
		}
		nfLaunch.Status.MainPod, _ = reference.GetReference(r.Scheme, &pod)

		nfLaunch.Status.Stage = statusRunning
		r.Status().Update(ctx, &nfLaunch)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NextflowLaunchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1alpha1.NextflowLaunch{}).
		Complete(r)
}
