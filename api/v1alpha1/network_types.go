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

package v1alpha1

import (
	"bytes"
	"context"
	"errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"net"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
)

const (
	ConditionInitialized    = "Initialized"
	ConditionFreeIPsUpdated = "FreeIPsUpdated"
	ConditionErrorNoFreeIPs = "ErrorNoFreeIPs"
	NetwotksAnnotation      = "networking.kettle.io/networks"
	StatusAnnotation        = "networking.kettle.io/status"
)

var (
	ErrIPAlreadyAllocated = errors.New("IP already allocated for Pod")
	ErrorNoIPsAvailable   = errors.New("no IP addresses available for allocation")
)

// NetworkSpec defines the desired state of Network
type NetworkSpec struct {
	// vlan is the VLAN ID of the network
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4096
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Vlan int `json:"vlan"`
	// cidr is the CIDR of the network this is a required field
	// +kubebuilder:validation:Pattern="^([0-9]{1,3}\\.){3}[0-9]{1,3}/[0-9]{1,2}$"
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	CIDR string `json:"cidr"`
	// gateway is the gateway of the network
	// +kubebuilder:validation:Pattern="^([0-9]{1,3}\\.){3}[0-9]{1,3}$"
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Gateway string `json:"gateway"`
	// nameServers is the list of nameservers for the network
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	NameServers []string `json:"nameServers,omitempty"`
	// ipRange is the range of IPs to assign to the network
	// +kubebuilder:validation:Pattern="^([0-9]{1,3}\\.){3}[0-9]{1,3}-([0-9]{1,3}\\.){3}[0-9]{1,3}$"
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	IPRange string `json:"ipRange"`
	// excludeIPs is the list of IPs to exclude from the IPRange
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ExcludeIPs []string `json:"excludeIPs,omitempty"`
	// routes is the list of routes for the network
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Routes []string `json:"routes,omitempty"`
}

// AllocatedIP represents list of allocated IPs in the network and it's associated Pods
type AllocatedIP struct {
	// IP is the IP address
	IP string `json:"ip"`
	// Name is the name of the pod
	PodName string `json:"podName"`
	// Namespace is the namespace of the pod
	Namespace string `json:"namespace"`
	// ifName is the name of the assigned interface
	IfName string `json:"ifName"`
}
type AllocatedIPMap map[types.NamespacedName]AllocatedIP

// NetworkStatus defines the observed state of Network
type NetworkStatus struct {
	// Represents the current state of a network
	// Network.Status.Conditions.Type are "Initialized" and "FreeIPsUpdated"
	// Network.Status.Conditions.Status are "True" and "False"
	// Network.Status.Conditions.Reason are camel case stings and producers of specific conditions types
	// Network.Status.Conditions.Message are human-readable messages that describe the condition

	// FeeIPs is the list of free IPs in the network
	// +operator-sdk:csv:customresourcedefinitions:type=status
	FreeIPs []string `json:"freeIPs,omitempty" protobuf:"bytes,1,rep,name=freeIPs"`
	// AllocatedIPs is the list of allocated IPs in the network
	// +operator-sdk:csv:customresourcedefinitions:type=status
	AssignedIPs AllocatedIPMap `json:"AssignedIPs,omitempty"  patchStrategy:"merge" patchMergeKey:"podUID" protobuf:"bytes,2,rep,name=AssignedIPs"`
	// Conditions is the list of conditions for the network
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,3,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="VLAN",type="integer",JSONPath=".spec.vlan",description="VLAN ID of the Network"
// +kubebuilder:printcolumn:name="CIDR",type="string",JSONPath=".spec.cidr",description="CIDR of the Network"
// +kubebuilder:printcolumn:name="Gateway",type="string",JSONPath=".spec.gateway",description="Gateway of the Network"

// Network is the Schema for the networks API
type Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkSpec   `json:"spec,omitempty"`
	Status NetworkStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NetworkList contains a list of Network
type NetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Network `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Network{}, &NetworkList{})
}

// Allocate takes a pod as an argument then checks the list of free IPs if there are free IPs then it grabs the first one in the list removing it and
// then updates status.freeIps, and status.assignedIps returning the ip address if successful
func (n *Network) Allocate(req ctrl.Request) (string, error) {
	if len(n.Status.FreeIPs) == 0 {
		return "", ErrorNoIPsAvailable
	}
	// check if the pod is already assigned an IP and raise an error if it is
	for namesapcedName, ip := range n.Status.AssignedIPs {
		if namesapcedName == req.NamespacedName {
			return ip.IP, ErrIPAlreadyAllocated
		}
	}
	// create a deep copy of the network status
	newStatus := n.Status.DeepCopy()
	// Get the first free IP
	ip := newStatus.FreeIPs[0]
	// Make a copy of the free IPs
	newStatus.FreeIPs = newStatus.FreeIPs[1:]
	// create a new allocated IP
	allocatedIP := AllocatedIP{
		IP:        ip,
		PodName:   req.Name,
		Namespace: req.Namespace,
	}
	// Add the IP to the list of assigned IPs
	if newStatus.AssignedIPs == nil {
		newStatus.AssignedIPs = make(AllocatedIPMap)
	}
	newStatus.AssignedIPs[req.NamespacedName] = allocatedIP // Update the status of the network
	n.Status = *newStatus

	return ip, nil
}

// Deallocate takes a pod as an argument then checks the list of assigned IPs if the pod is assigned an IP then it removes it from the list
// and adds it back to the free IPs list
func (n *Network) Deallocate(req ctrl.Request) error {
	// Todo: Should this be an error? I think there are some cases where it should and some where it shouldn't
	// 	- If the pod is not found, it should not be an error
	// 	- If the pod is found but does not have an IP, it should not be an error
	// 	- If the pod is found and has an IP, it should not be an error
	// 	- If the pod is found and has an IP but the IP is not in the assigned list, it should be an error
	logger := log.FromContext(context.Background()).WithCallDepth(0).WithValues("pod", req.Name, "namespace", req.Namespace)
	logger.Info("Deallocating IP for pod")
	// create a deep copy of the network status
	newStatus := n.Status.DeepCopy()
	// Loop through the assigned IPs
	for namespacedName, ip := range newStatus.AssignedIPs {
		if namespacedName == req.NamespacedName {
			logger.Info("Checking if the pod is assigned the IP", "status.namespacedName", namespacedName, "pod", req.Name, "status.ip", ip.IP)
			// Remove the IP from the list of assigned IPs
			delete(newStatus.AssignedIPs, namespacedName)
			// insert the IP back to the free IPs list
			newStatus.FreeIPs = append(newStatus.FreeIPs, ip.IP)
			// Update the status of the network
			n.Status = *newStatus
			return nil
		}
	}
	return errors.New("Unable to deallocate IP in pod: " + req.Name + " in namespace: " + req.Namespace)
}

// GetStatusAnnotation returns the status annotation for the network with all the network details as a string
func (n *Network) GetStatusAnnotation(ip string) (string, error) {
	if n.Spec.NameServers == nil {
		n.Spec.NameServers = []string{}
	}
	if n.Spec.Routes == nil {
		n.Spec.Routes = []string{}
	}
	networkDetails := map[string]interface{}{
		"VLAN":        n.Spec.Vlan,
		"CIDR":        n.Spec.CIDR,
		"Gateway":     n.Spec.Gateway,
		"IP":          ip,
		"nameServers": n.Spec.NameServers,
		"routes":      n.Spec.Routes,
	}

	// convert map to JSON
	jsonBytes, err := json.Marshal(networkDetails)
	if err != nil {
		return "", err
	}
	// convert the JSON bytes to a string
	return string(jsonBytes), nil
}

// UpdateCondition is a helper function to update the condition of the network making sure that the existing conditions
// are not replaced or duplicated
func (n *Network) UpdateCondition(newCondition metav1.Condition) {
	updated := false
	for i, condition := range n.Status.Conditions {
		if condition.Type == newCondition.Type {
			n.Status.Conditions[i] = newCondition
			updated = true
			break
		}
	}
	if !updated {
		n.Status.Conditions = append(n.Status.Conditions, newCondition)
	}
}

// SetConditionInitialized checks is the initialized condition is present and sets it to the given status
func (n *Network) SetConditionInitialized(status metav1.ConditionStatus) {
	var message string
	var reason string
	if status == metav1.ConditionTrue {
		message = "Network has been initialized"
		reason = "NetworkInitialized"
	} else {
		message = "Network has not been initialized"
		reason = "NetworkNotInitialized"
	}

	n.UpdateCondition(metav1.Condition{
		Type:               ConditionInitialized,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetConditionFreeIPsUpdated checks is the FreeIPsUpdated condition is present and sets it to the given status
func (n *Network) SetConditionFreeIPsUpdated(status metav1.ConditionStatus) {
	n.UpdateCondition(metav1.Condition{
		Type:               ConditionFreeIPsUpdated,
		Status:             status,
		Reason:             "FreeIPsUpdated",
		Message:            "FreeIPs have been updated",
		LastTransitionTime: metav1.Now(),
	})
}

// SetConditionErrorNoFreeIPs checks is the ErrorNoFreeIPs condition is present and sets it to the given status
func (n *Network) SetConditionErrorNoFreeIPs(status metav1.ConditionStatus) {
	n.UpdateCondition(metav1.Condition{
		Type:               ConditionErrorNoFreeIPs,
		Status:             status,
		Reason:             "NoFreeIPs",
		Message:            "No free IPs available",
		LastTransitionTime: metav1.Now(),
	})
}

// IsConditionPresentAndEqual checks if the condition is present and equal to the given status
func (n *Network) IsConditionPresentAndEqual(conditionType string, status metav1.ConditionStatus) bool {
	for _, condition := range n.Status.Conditions {
		if condition.Type == conditionType && condition.Status == status {
			return true
		}
	}
	return false
}

// GetIPs generates a list IPs based on the given NetworkSpec. It excludes the gateway, network, broadcast addresses,
// and any IPs in the exclude list. It also filters IPs based on the IP range if provided.
func (n *Network) GetIPs() ([]string, error) {
	var allocatableIPs []string
	if n.Spec.CIDR == "" {
		return nil, errors.New("spec.cidr is required")
	}
	// Parse the CIDR to generate allocatable IPs
	_, cidr, err := net.ParseCIDR(n.Spec.CIDR)
	if err != nil {
		return nil, errors.New("invalid CIDR format")
	}
	var startIP, endIP net.IP = nil, nil

	if n.Spec.IPRange != "" {
		// Get the start and end IPs of the range
		startIP, endIP, err = n.GetRangeIPs()
		if err != nil {
			return nil, err
		}
	}

	// Generate a list of allocatable IPs from CIDR
	for ip := cidr.IP.Mask(cidr.Mask); cidr.Contains(ip); incrementIP(ip) {
		// Convert the IP to a string
		ipStr := ip.String()
		// exit the loop if the ip address is the broadcast address
		if ipStr == broadcastAddress(cidr) {
			break
		}
		// Skip network, broadcast addresses, gateway, and exclude IPs
		if ipStr != cidr.IP.String() &&
			ipStr != n.Spec.Gateway &&
			!contains(n.Spec.ExcludeIPs, ipStr) &&
			inRange(ip, startIP, endIP) {

			allocatableIPs = append(allocatableIPs, ipStr)
		}
	}

	return allocatableIPs, nil
}

// GetRangeIPs returns the first and last IP of the range as a Net.IP
func (n *Network) GetRangeIPs() (net.IP, net.IP, error) {
	// Split the IP range
	ipRange := strings.Split(n.Spec.IPRange, "-")
	if len(ipRange) != 2 {
		return nil, nil, errors.New("invalid IP range format")
	}
	// Parse the start and end IPs
	startIP := net.ParseIP(ipRange[0])
	endIP := net.ParseIP(ipRange[1])
	if startIP == nil || endIP == nil {
		return nil, nil, errors.New("invalid IP range format")
	}
	return startIP, endIP, nil
}

// IPReconcile check all the AssignedIps are not present in the FreeIPs

// ShouldReconcile checks if the Network should be reconciled based on the status of the given Network
// and the current Network
func (n *Network) ShouldReconcile(newNetwork *Network) bool {
	// Todo this needs it's own unit tests
	logger := log.FromContext(context.Background()).WithCallDepth(3)

	if len(n.Status.AssignedIPs) != len(newNetwork.Status.AssignedIPs) {
		logger.Info("Allocated IPs are not the same length")
		return true
	}

	for namespacedName, assignedIP := range n.Status.AssignedIPs {
		newAssignedIP, exists := newNetwork.Status.AssignedIPs[namespacedName]
		if !exists || assignedIP.IP != newAssignedIP.IP {
			logger.Info("IPs are not equal", "oldIP", assignedIP.IP, "newIP", newAssignedIP.IP)
			return true
		}
		if assignedIP.PodName != newAssignedIP.PodName && assignedIP.Namespace != newAssignedIP.Namespace {
			logger.Info("PodNames are not equal", "oldPodName", assignedIP.PodName, "newPodName", newAssignedIP.PodName)
			return true
		}
	}
	logger.Info("Filtered event", "newNetwork", newNetwork.Name, "currentNetwork", n.Name)
	return false
}

// incrementIP increments the given IP address by 1
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// contains checks if a given slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// broadcastAddress calculates the broadcast address for a given CIDR
func broadcastAddress(cidr *net.IPNet) string {
	ip := cidr.IP
	for i := range ip {
		ip[i] |= ^cidr.Mask[i]
	}
	return ip.String()
}

// inRange checks if an IP address is within a given range
func inRange(ip net.IP, start net.IP, end net.IP) bool {
	// start and end will be nil if the IP range is not provided so always return true
	if start == nil || end == nil {
		return true
	}
	// Check if the IP is greater than or equal to the start IP and less than or equal to the end IP
	// making sure to convert the IPs to 16 byte format. This should support both ipv4 and ipv6 addresses
	if bytes.Compare(ip.To16(), start.To16()) >= 0 && bytes.Compare(ip.To16(), end.To16()) <= 0 {
		return true
	}
	return false
}
