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
	"net"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
)

const (
	ConditionInitialized    = "Initialized"
	ConditionFreeIPsUpdated = "FreeIPsUpdated"
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
}

// AllocatedIP represents list of allocated IPs in the network and it's associated Pods
type AllocatedIP struct {
	// IP is the IP address
	IP string `json:"ip"`
	// Name is the name of the pod
	PodName string `json:"podName"`
	// Namespace is the namespace of the pod
	Namespace string `json:"namespace"`
	// PodUID is the UID of the pod
	PodUID types.UID `json:"podUID"`
	// ifName is the name of the assigned interface
	IfName string `json:"ifName"`
}

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
	AssignedIPs []AllocatedIP `json:"AssignedIPs,omitempty"  patchStrategy:"merge" patchMergeKey:"podUID" protobuf:"bytes,2,rep,name=AssignedIPs"`
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
	logger := log.FromContext(context.Background()).WithCallDepth(3)

	if len(n.Status.AssignedIPs) != len(newNetwork.Status.AssignedIPs) {
		logger.Info("Allocated IPs are not the same length")
		return true
	}

	for i := range n.Status.AssignedIPs {
		if n.Status.AssignedIPs[i].IP != newNetwork.Status.AssignedIPs[i].IP {
			logger.Info("IPs are not equal " + n.Status.AssignedIPs[i].IP + " " + newNetwork.Status.AssignedIPs[i].IP)
			return true
		}
		if n.Status.AssignedIPs[i].PodName != newNetwork.Status.AssignedIPs[i].PodName {
			logger.Info("PodNames are not equal")
			return true
		}
		if n.Status.AssignedIPs[i].PodUID != newNetwork.Status.AssignedIPs[i].PodUID {
			logger.Info("PodUIDs are not equal")
			return true
		}
	}
	logger.Info("Filtered event", "newNetwork", newNetwork.Name, "newNetwork", n.Name)
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
