package v1alpha1_test

import (
	"github.com/jdambly/kettle/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Network GetIPs", func() {
	var (
		network    *v1alpha1.Network
		newNetwork *v1alpha1.Network
	)

	BeforeEach(func() {
		network = &v1alpha1.Network{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-network",
				Namespace: "default",
			},
			Spec: v1alpha1.NetworkSpec{
				CIDR:       "10.0.0.0/24",
				Gateway:    "10.0.0.1",
				ExcludeIPs: []string{"10.0.0.5", "10.0.0.10"},
			},
			Status: v1alpha1.NetworkStatus{},
		}
		newNetwork = network.DeepCopy()
	})

	Context("when CIDR is provided", func() {
		BeforeEach(func() {
			network.Spec.CIDR = "10.0.1.0/29"
			network.Spec.Gateway = "10.0.1.1"
		})

		It("should generate allocatable IPs based on the CIDR and apply exclusions", func() {
			allocatableIPs, err := network.GetIPs()
			Expect(err).ToNot(HaveOccurred())
			Expect(allocatableIPs).To(ContainElement("10.0.1.2"))
			Expect(allocatableIPs).To(ContainElement("10.0.1.6"))
			Expect(allocatableIPs).ToNot(ContainElement("10.0.1.1"))
		})
	})

	Context("when IPRange is provided", func() {
		BeforeEach(func() {
			network.Spec.CIDR = "10.0.0.0/24"
			network.Spec.IPRange = "10.0.0.20-10.0.0.30"
		})

		It("should generate allocatable IPs based on the given range and apply exclusions", func() {
			allocatableIPs, err := network.GetIPs()
			Expect(err).ToNot(HaveOccurred())
			Expect(allocatableIPs).To(ContainElement("10.0.0.20"))
			Expect(allocatableIPs).To(ContainElement("10.0.0.25"))
			Expect(allocatableIPs).To(ContainElement("10.0.0.30"))
			Expect(allocatableIPs).ToNot(ContainElement("10.0.0.5"))
			Expect(allocatableIPs).ToNot(ContainElement("10.0.0.10"))
		})
	})

	Context("when IPRange and CIDR are missing", func() {
		BeforeEach(func() {
			network.Spec.IPRange = ""
			network.Spec.CIDR = ""
		})

		It("should return an empty list of allocatable IPs", func() {
			allocatableIPs, err := network.GetIPs()
			Expect(err).To(HaveOccurred())
			Expect(allocatableIPs).To(BeEmpty())
		})
	})
	Context("When a conditions are set", func() {
		BeforeEach(func() {
			network.SetConditionInitialized(metav1.ConditionTrue)
			network.SetConditionFreeIPsUpdated(metav1.ConditionTrue)
		})
		It("Should have both conditions set to true", func() {
			Expect(network.Status.Conditions).To(HaveLen(2))
			Expect(network.Status.Conditions[0].Type).To(Equal(v1alpha1.ConditionInitialized))
			Expect(network.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(network.Status.Conditions[1].Type).To(Equal(v1alpha1.ConditionFreeIPsUpdated))
			Expect(network.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When assigned ip has been detected", func() {
		BeforeEach(func() {
			network.Status.AssignedIPs = []v1alpha1.AllocatedIP{
				{IP: "10.1.0.2", PodName: "test-pod1", PodUID: "1234"},
				{IP: "10.1.0.3", PodName: "test-pod2", PodUID: "1235"},
			}
			newNetwork.Status.AssignedIPs = []v1alpha1.AllocatedIP{
				{IP: "10.1.0.2", PodName: "test-pod1", PodUID: "1234"},
				{IP: "10.1.0.3", PodName: "test-pod2", PodUID: "1235"},
			}
			allocatableIPs, err := network.GetIPs()
			newAllocatableIPs, newErr := newNetwork.GetIPs()
			network.Status.FreeIPs = allocatableIPs
			newNetwork.Status.FreeIPs = newAllocatableIPs

			Expect(err).ToNot(HaveOccurred())
			Expect(newErr).ToNot(HaveOccurred())
			Expect(allocatableIPs).To(Equal(newAllocatableIPs))

		})
		It("Returns false when the status are Equal", func() {
			Expect(network.ShouldReconcile(newNetwork)).To(BeFalse())
		})
		It("Returns true when the status are not Equal", func() {
			newNetwork.Status.AssignedIPs = []v1alpha1.AllocatedIP{
				{IP: "10.1.0.2", PodName: "test-pod3", PodUID: "1236"},
			}
			Expect(network.ShouldReconcile(newNetwork)).To(BeTrue())
		})
		It("Return true when there is duplicate ips", func() {
			newNetwork.Status.AssignedIPs = []v1alpha1.AllocatedIP{
				{IP: "10.1.0.2", PodName: "test-pod3", PodUID: "1236"},
			}
			Expect(network.ShouldReconcile(newNetwork)).To(BeTrue())
		})
	})
	Context("When allocating an IP to a pod", func() {
		var pod *corev1.Pod

		BeforeEach(func() {
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "1234",
				},
			}
			network.Status.FreeIPs = []string{"10.0.0.2", "10.0.0.3"}
		})

		It("should allocate the first free IP to the pod", func() {
			_, err := network.Allocate(pod)
			Expect(err).ToNot(HaveOccurred())
			Expect(network.Status.FreeIPs).ToNot(ContainElement("10.0.0.2"))
			Expect(network.Status.AssignedIPs).To(ContainElement(v1alpha1.AllocatedIP{
				IP:        "10.0.0.2",
				PodName:   "test-pod",
				Namespace: "default",
				PodUID:    "1234",
			}))
		})

		It("should return an error if no free IPs are available", func() {
			network.Status.FreeIPs = []string{}
			_, err := network.Allocate(pod)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("no free IPs available"))
		})
	})
	Context("When deallocating an IP from a pod", func() {
		var pod *corev1.Pod

		BeforeEach(func() {
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "1234",
				},
			}
			network.Status.AssignedIPs = []v1alpha1.AllocatedIP{
				{IP: "10.0.0.2", PodName: "test-pod", Namespace: "default", PodUID: "1234"},
			}
			network.Status.FreeIPs = []string{}
		})

		It("should deallocate the IP from the pod and add it back to the free IPs list", func() {
			network.Deallocate(pod)
			Expect(network.Status.AssignedIPs).To(BeEmpty())
			Expect(network.Status.FreeIPs).To(ContainElement("10.0.0.2"))
		})

		It("should do nothing if the pod does not have an assigned IP", func() {
			pod.UID = "5678"
			network.Deallocate(pod)
			Expect(network.Status.AssignedIPs).To(HaveLen(1))
			Expect(network.Status.FreeIPs).To(BeEmpty())
		})
	})
})
