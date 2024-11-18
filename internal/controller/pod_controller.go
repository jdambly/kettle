/*
Copyright 2024.

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

package controller

import (
	"context"
	ipamv1alpha1 "github.com/jdambly/kettle/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=pods/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithCallDepth(3).WithValues("pod", req.NamespacedName)
	// todo just logging now for testing purposes to make sure the predicate funcs are working
	logger.Info("Reconciling Pod")
	pod := &corev1.Pod{}
	err := r.Get(ctx, req.NamespacedName, pod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected, we need to garbage collect
			// the ip addresses
			// todo (jdambly): implement garbage collection, this involves removing the ip address from the network
			// and updating the network status
			return ctrl.Result{}, nil
		}
	}
	// Get the value of the network annotation
	netAnnotaionValue, ok := pod.GetAnnotations()[ipamv1alpha1.NetwotksAnnotation]
	if !ok {
		logger.Error(err, "failed to get network annotation")
		return ctrl.Result{}, err
	}
	// create a client.ObjectKey for the network
	netReq := client.ObjectKey{
		Name:      netAnnotaionValue,
		Namespace: "", // Network objects are not namespaced
	}
	network := &ipamv1alpha1.Network{}
	err = r.Get(ctx, netReq, network)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Network not found, return. This can be ignored.
			logger.Info("Network not found, ignoring", "network", netAnnotaionValue)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get network", "network", netAnnotaionValue)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc:  updatedFilter,
			CreateFunc:  createFilter,
			DeleteFunc:  deleteFilter,
			GenericFunc: genericFilter,
		}).
		Complete(r)
}

// updateFilter if a helper function used for predicate.Funcs to determine if a pod should be reconciled. updates are ingnored
func updatedFilter(e event.UpdateEvent) bool {
	// pod updates are ignored
	return false
}

// createFilter if a helper function used for predicate.Funcs to determine if a pod should be reconciled. this func
// will only return true if the pod has the annotations for network
func createFilter(e event.CreateEvent) bool {
	_, hasAnnotation := e.Object.GetAnnotations()[ipamv1alpha1.NetwotksAnnotation]
	return hasAnnotation
}

// deleteFilter if a helper function used for predicate.Funcs to determine if a pod should be reconciled. this func
// will only return true if the pod has the annotations for network or status
func deleteFilter(e event.DeleteEvent) bool {
	_, hasNetAnnotation := e.Object.GetAnnotations()[ipamv1alpha1.NetwotksAnnotation]
	_, hasStatusAnnotation := e.Object.GetAnnotations()[ipamv1alpha1.StatusAnnotation]
	return hasStatusAnnotation || hasNetAnnotation
}

// genericFilter if a helper function used for predicate.Funcs to determine if a pod should be reconciled. this func
// returns false to filter out all generic events
func genericFilter(e event.GenericEvent) bool {
	return false
}
