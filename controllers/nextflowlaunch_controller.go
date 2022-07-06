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
	"context"
	"math/rand"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	batchv1alpha1 "mnmdiagnostics/nextflowop/api/v1alpha1"
)

// NextflowLaunchReconciler reconciles a NextflowLaunch object
type NextflowLaunchReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	defaultNextflowImage   = "nextflow/nextflow"
	defaultNextflowVersion = "22.06.0-edge"
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

// Retrieve the NextflowLaunch object's child pod
func getChildPod(r *NextflowLaunchReconciler, ctx context.Context, nfLaunch batchv1alpha1.NextflowLaunch) (corev1.Pod, error) {

	var pod corev1.Pod
	podName := types.NamespacedName{
		Namespace: nfLaunch.Status.MainPod.Namespace,
		Name:      nfLaunch.Status.MainPod.Name,
	}
	err := r.Get(ctx, podName, &pod)
	return pod, err
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
	return corev1.Pod{
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
							ClaimName: "rnasexi-pvc", // FIXME: hard-coded name
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}

// Retrieve the NextflowLaunch object's config map
func getConfigMap(r *NextflowLaunchReconciler, ctx context.Context, nfLaunch batchv1alpha1.NextflowLaunch) (corev1.ConfigMap, error) {

	var configMap corev1.ConfigMap
	mapName := types.NamespacedName{
		Namespace: nfLaunch.Status.ConfigMap.Namespace,
		Name:      nfLaunch.Status.ConfigMap.Name,
	}
	err := r.Get(ctx, mapName, &configMap)
	return configMap, err
}

// Construct a Nextflow config file as a ConfigMap
func makeNextflowConfig(nfLaunch batchv1alpha1.NextflowLaunch) corev1.ConfigMap {
	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nfLaunch.Name + "-nextflow-config-" + generateHash(8),
			Namespace: nfLaunch.Namespace,
		},
		Data: map[string]string{
			"nextflow.config": `process {
                  executor = 'k8s'
               }
               k8s {
                  //serviceAccount = 'nextflow-sa'
                  storageClaimName = 'rnasexi-pvc'
               }`, // FIXME: hard-coded values
		},
	}
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
		log.Error(err, "error fetching Nextflow launch "+req.Name)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	stage := nfLaunch.Status.Stage

	if stage == "Running" {
		// job is running, check status
		log.Info("Job running")
		pod, err := getChildPod(r, ctx, nfLaunch)
		if err != nil {
			log.Error(err, "error fetching pod")
			return ctrl.Result{}, err
		}
		status := pod.Status.Phase
		log.Info(string(status))
		if status == corev1.PodSucceeded {
			nfLaunch.Status.Stage = "Succeeded"
			r.Status().Update(ctx, &nfLaunch)
		} else if status == corev1.PodFailed {
			nfLaunch.Status.Stage = "Failed"
			r.Status().Update(ctx, &nfLaunch)
		} else {
			// come revisit later
			return ctrl.Result{RequeueAfter: 3e+9}, nil
		}

	} else if stage == "Succeeded" {
		// job has finished successfully
		log.Info("Job finished.")
		// remove child pod
		// TODO: should the pod be deleted on success??
		// TODO: (everything sent to stdout/stderr will be lost)
		pod, err := getChildPod(r, ctx, nfLaunch)
		if err == nil {
			err = r.Delete(ctx, &pod)
			if err != nil {
				log.Info("Couldn't remove pod " + pod.Name)
			}
			nfLaunch.Status.MainPod = &corev1.ObjectReference{}
			log.Info("Successfully removed pod " + pod.Name)
		}
		// remove config map
		configMap, err := getConfigMap(r, ctx, nfLaunch)
		if err == nil {
			r.Delete(ctx, &configMap)
		}
		nfLaunch.Status.ConfigMap = &corev1.ObjectReference{}
		r.Status().Update(ctx, &nfLaunch)

	} else if stage == "Failed" {
		// job has failed
		log.Info("Job failed! Use `kubectl describe pod " +
			nfLaunch.Status.MainPod.Name +
			"` to diagnose")

	} else {
		// job is ready to run
		configMap := makeNextflowConfig(nfLaunch)
		err = r.Client.Create(ctx, &configMap)
		if err != nil {
			log.Error(err, "error creating Nextflow config")
			return ctrl.Result{}, err
		}
		nfLaunch.Status.ConfigMap, _ = reference.GetReference(r.Scheme, &configMap)
		pod := makeNextflowPod(nfLaunch, configMap.Name)
		log.Info("Starting job " + pod.Name)
		err = r.Client.Create(ctx, &pod)
		if err != nil {
			log.Error(err, "error creating pod")
			return ctrl.Result{}, err
		}
		nfLaunch.Status.MainPod, _ = reference.GetReference(r.Scheme, &pod)
		// TODO: how to get the child pod auto-removed after the launch is deleted?
		ctrl.SetControllerReference(&nfLaunch, &pod, r.Scheme)
		ctrl.SetControllerReference(&nfLaunch, &configMap, r.Scheme)
		nfLaunch.Status.Stage = "Running"
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
