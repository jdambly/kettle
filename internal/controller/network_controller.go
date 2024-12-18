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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ipamv1alpha1 "github.com/jdambly/kettle/api/v1alpha1"
)

// NetworkReconciler reconciles a Network object
type NetworkReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Todo create a NewReconciler function that returns a new NetworkReconciler, this will be used in the main.go file

//+kubebuilder:rbac:groups=ipam.kettle.io,resources=networks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.kettle.io,resources=networks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.kettle.io,resources=networks/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Network object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *NetworkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// todo (jdambly): investigate how to enable the logging so it's initialized once not in every reconcile
	logger := log.FromContext(ctx).WithCallDepth(0).WithValues("network", req.NamespacedName)

	// Fetch the Network instance
	network := &ipamv1alpha1.Network{}
	err := r.Get(ctx, req.NamespacedName, network)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Network resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Network")
		return ctrl.Result{}, err
	}

	// Make a copy of the current status
	originalStatus := network.Status.DeepCopy()

	// Initialize the Network if needed
	if !network.IsConditionPresentAndEqual(ipamv1alpha1.ConditionInitialized, metav1.ConditionTrue) {
		logger.Info("Network not initialized, initializing...")
		r.Initialize(network)
	} else {
		logger.Info("Network already initialized, skipping...")
	}

	// Compare the original status with the updated status
	if !reflect.DeepEqual(originalStatus, &network.Status) {
		logger.Info("Status has changed, updating...")
		// Update the status of the resource
		if err := r.Status().Update(ctx, network); err != nil {
			logger.Error(err, "Failed to update Network status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// Initialize checks if the Network is initialized and if not allocates ip addresses and sets the Initialized condition
func (r *NetworkReconciler) Initialize(network *ipamv1alpha1.Network) {

	// Allocate IP addresses
	allocatableIPs, err := network.GetIPs()
	if err != nil {
		// set the Initialized condition to False
		network.SetConditionInitialized(metav1.ConditionFalse)
	}

	// Update the Network status with the allocated IP addresses
	network.Status.FreeIPs = allocatableIPs
	network.Status.AssignedIPs = make(ipamv1alpha1.AllocatedIPMap)

	// Set the Initialized condition to True
	network.SetConditionInitialized(metav1.ConditionTrue)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// create a predicate to only trigger the controller when the status is updated with changes to allocated IPs
	// this is to avoid unnecessary reconciliations
	statusUpdatePredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj := e.ObjectOld.(*ipamv1alpha1.Network)
			newObj := e.ObjectNew.(*ipamv1alpha1.Network)
			return oldObj.ShouldReconcile(newObj)
		},
	}
	// predicate is used here to make sure the controller triggers when the status is updated
	// it should also trigger on create/update/delete events
	return ctrl.NewControllerManagedBy(mgr).
		For(&ipamv1alpha1.Network{}).
		WithEventFilter(statusUpdatePredicate).
		Complete(r)
}
