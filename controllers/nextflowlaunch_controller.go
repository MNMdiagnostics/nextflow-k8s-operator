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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

//+kubebuilder:rbac:groups=batch.mnm.bio,resources=nextflowlaunches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch.mnm.bio,resources=nextflowlaunches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch.mnm.bio,resources=nextflowlaunches/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

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

	nfLaunch, err = validateLaunch(nfLaunch)
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
			if nfLaunch.Status.Launched {
				// driver pod has been killed, recreate session
				nfLaunch.Status.Stage = statusRelaunch
				nfLaunch.Status.Launched = false
				r.Status().Update(ctx, &nfLaunch)
				log.Info("Driver pod disappeared. Relaunching...")
				return ctrl.Result{RequeueAfter: 5e+9}, nil
			} else {
				log.Error(err, "Error fetching driver pod")
				return ctrl.Result{}, err
			}
		}
		status := pod.Status.Phase
		log.Info("Job running (" + string(status) + ")")

		// pod running? mark as successful launch
		if (!nfLaunch.Status.Launched) && (status == corev1.PodRunning) {
			nfLaunch.Status.Launched = true
			r.Status().Update(ctx, &nfLaunch)
		}

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
