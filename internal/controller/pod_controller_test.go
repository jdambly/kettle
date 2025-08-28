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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Pod Controller", func() {
	Context("When reconciling a pod resource", func() {
		const resourceName = "test-resource"
		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		pod := &corev1.Pod{}
		BeforeEach(func() {
			By("creating a pod resource and a network resource")
			err := k8sClient.Get(ctx, typeNamespacedName, pod)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
						Annotations: map[string]string{
							ipamv1alpha1.NetworksAnnotation: "network1",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test-container",
								Image: "test-image",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).Should(Succeed())
			}
		})
		AfterEach(func() {
			pod := &corev1.Pod{}
			err := k8sClient.Get(ctx, typeNamespacedName, pod)
			Expect(err).ToNot(HaveOccurred())
			By("deleting the pod resource")
			Expect(k8sClient.Delete(ctx, pod)).Should(Succeed())

		})

		It("should successfully reconcile the resource", func() {
			By("reconciling the pod resource with our annotation")
			podReconciler := &PodReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := podReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
