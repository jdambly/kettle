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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kettlev1alpha1 "github.com/jdambly/kettle/api/v1alpha1"
)

var _ = Describe("Network Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		network := &kettlev1alpha1.Network{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Network")
			err := k8sClient.Get(ctx, typeNamespacedName, network)
			if err != nil && errors.IsNotFound(err) {
				resource := &kettlev1alpha1.Network{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: kettlev1alpha1.NetworkSpec{
						Vlan:        100,
						CIDR:        "10.0.0.0/24",
						Gateway:     "10.0.0.1",
						NameServers: nil,
						IPRange:     "10.0.0.10-10.0.0.20",
						ExcludeIPs:  []string{"10.0.0.11", "10.0.0.12"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &kettlev1alpha1.Network{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Network")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &NetworkReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the resource has been initialized")
			err = k8sClient.Get(ctx, typeNamespacedName, network)
			Expect(err).NotTo(HaveOccurred())

			By("checking that status has been updated with the list of free ips")
			Expect(network.Status.FreeIPs).To(ContainElement("10.0.0.10"))
			Expect(network.Status.FreeIPs).To(ContainElement("10.0.0.20"))
			Expect(len(network.Status.FreeIPs)).To(Equal(9))
		})
		It("should detect updates to the status fields and reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &NetworkReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the resource has been initialized")
			err = k8sClient.Get(ctx, typeNamespacedName, network)
			Expect(err).NotTo(HaveOccurred())

		})
	})
})
