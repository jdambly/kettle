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
	"errors"
	"fmt"
	ipamv1alpha1 "github.com/jdambly/kettle/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
	logger := log.FromContext(ctx).WithCallDepth(0).WithValues("pod", req.NamespacedName)
	logger.Info("Reconciling Pod", "namespace", req.Namespace, "name", req.Name)
	pod := &corev1.Pod{}
	// to properly garbage collect the ip address we need to get the pod and the errors needs to persist into the code
	// to make sure it's handled properly, we don't want to ignore the error or have the garbage collection fail silently
	// if another error occurs
	errPod := r.Get(ctx, req.NamespacedName, pod)
	logger.Info("Got pod", "pod", pod, "UID", pod.UID, "error", errPod)
	// Get the value of the network annotation
	netAnnotaionValue, ok := pod.GetAnnotations()[ipamv1alpha1.NetwotksAnnotation]
	if !ok {
		// Todo: fix this error message "Operation cannot be fulfilled on pods \"busybox-pod\": the object has been modified; please apply your changes to the latest version and try again"
		logger.Error(errPod, "failed to get network annotation", "annotation", netAnnotaionValue)
		// Todo: some cleanup logic is needed here there could be a case were the network annotation is removed and we missed the update, check if there is a status entry for this pod and remove it
		return ctrl.Result{}, errPod
	}
	// create a deep copy of the pod to avoid modifying the original object
	updatedPod := pod.DeepCopy()

	// create a client.ObjectKey for the network
	netReq := client.ObjectKey{
		Name:      netAnnotaionValue,
		Namespace: "", // Network objects are not namespaced
	}
	// Get the network object
	network := &ipamv1alpha1.Network{}
	err := r.Get(ctx, netReq, network)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Network not found, return. This can be ignored.
			logger.Info("Pod annotation is set but no network was found", "network", netAnnotaionValue)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get network", "network", netAnnotaionValue)
		return ctrl.Result{}, err
	}
	// check network.Status.Conditions to see if the network is ready
	if !network.IsConditionPresentAndEqual(ipamv1alpha1.ConditionInitialized, metav1.ConditionTrue) {
		// Network is not ready, request a requeue
		logger.Info("Network is not ready", "network", netAnnotaionValue)
		return ctrl.Result{Requeue: true}, nil
	}
	// create a deep copy of the network to avoid modifying the original object
	udpatedNetwork := network.DeepCopy()
	if errPod != nil {
		if apierrors.IsNotFound(errPod) {
			// Object not found, return.  Created objects are automatically garbage collected, we need to garbage collect
			// the ip addresses and update the network status
			_ = udpatedNetwork.Deallocate(pod)
			// update the status of the network
			err := r.Status().Patch(ctx, udpatedNetwork, client.MergeFrom(network))
			if err != nil {
				if apierrors.IsConflict(err) {
					logger.Info("Conflict while updating network, retrying", "error", err, "network", udpatedNetwork.Name)
					return ctrl.Result{Requeue: true}, nil
				}
				logger.Error(err, "failed to update network status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}
	// allocate the ip address to the pod
	ip, err := network.Allocate(pod)
	if errors.Is(err, ipamv1alpha1.ErrorNoIPsAvailable) {
		// No allocatable IPs, request a requeue
		// todo: try to garbage collect first
		logger.Error(err, "network", netAnnotaionValue)
		network.SetConditionFreeIPsUpdated(metav1.ConditionTrue)
		return ctrl.Result{Requeue: false}, nil
	}
	logger.Info("Allocated IP address to pod", "ip", ip)
	// update the network status
	err = r.Status().Patch(ctx, udpatedNetwork, client.MergeFrom(network))
	if err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Conflict while updating network, retrying", "error", err, "network", udpatedNetwork.Name)
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update network status")
		return ctrl.Result{}, err
	}
	// Update the pod with the network information annotation
	statusAnnotation, err := network.GetStatusAnnotation(ip)
	if err != nil {
		logger.Error(err, "failed to get network status annotation")
		return ctrl.Result{}, err
	}
	logger.Info("Updating pod with network annotation", "annotation", statusAnnotation)
	podAnnotations := map[string]string{
		ipamv1alpha1.StatusAnnotation: statusAnnotation,
	}
	// before setting the annotations we need to make sure we are not overwriting any existing annotations so make a copy
	// of the existing annotations
	for k, v := range pod.GetAnnotations() {
		podAnnotations[k] = v

	}
	updatedPod.SetAnnotations(podAnnotations)
	// Apply a patch using the MergeFrom helper function
	if err := r.Patch(ctx, updatedPod, client.MergeFrom(pod)); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Conflict while updating pod, retrying", "error", err, "pod", updatedPod.Name, "namespace", updatedPod.Namespace)
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed to update pod with network annotation", "annotation", statusAnnotation, "pod", updatedPod.Name, "namespace", updatedPod.Namespace)
		return ctrl.Result{}, err
	}

	logger.Info("Patched pod with network annotation", "annotation", statusAnnotation)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Add an index to help list Pods based on the network annotation.
	// typically this is done for you but since we are watching objects that are not the primary resource
	// we need to create the index ourselves
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, ipamv1alpha1.NetwotksAnnotation, func(rawObj client.Object) []string {
		pod := rawObj.(*corev1.Pod)
		if network, exists := pod.Annotations[ipamv1alpha1.NetwotksAnnotation]; exists {
			return []string{network}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to create field index for pod annotations: %v", err)
	}

	// Create a new controller managed by the manager using watches for pods and f
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

// updateFilter if a helper function used for predicate.Funcs to determine if a pod should be reconciled. updates are ignored
func updatedFilter(e event.UpdateEvent) bool {
	// pod updates are ignored
	return false
}

// createFilter if a helper function used for predicate.Funcs to determine if a pod should be reconciled. this func
// will only return true if the pod has the annotations for network
func createFilter(e event.CreateEvent) bool {
	logger := log.FromContext(context.Background()).WithCallDepth(0).WithValues("pod", e.Object.GetName(), "namespace", e.Object.GetNamespace())
	_, hasAnnotation := e.Object.GetAnnotations()[ipamv1alpha1.NetwotksAnnotation]
	logger.Info("Create event", "hasAnnotation", hasAnnotation)
	return hasAnnotation
}

// deleteFilter if a helper function used for predicate.Funcs to determine if a pod should be reconciled. this func
// will only return true if the pod has the annotations for network or status
func deleteFilter(e event.DeleteEvent) bool {
	logger := log.FromContext(context.Background()).WithCallDepth(3).WithValues("pod", e.Object.GetName(), "namespace", e.Object.GetNamespace())
	logger.Info("Delete event", "pod", e.Object.GetName(), "namespace", e.Object.GetNamespace())
	_, hasNetAnnotation := e.Object.GetAnnotations()[ipamv1alpha1.NetwotksAnnotation]
	_, hasStatusAnnotation := e.Object.GetAnnotations()[ipamv1alpha1.StatusAnnotation]
	logger.Info("Delete event", "hasNetAnnotation", hasNetAnnotation, "hasStatusAnnotation", hasStatusAnnotation, "pod", e.Object.GetName(), "namespace", e.Object.GetNamespace())
	return hasStatusAnnotation || hasNetAnnotation
}

// genericFilter if a helper function used for predicate.Funcs to determine if a pod should be reconciled. this func
// returns false to filter out all generic events
func genericFilter(e event.GenericEvent) bool {
	return false
}
