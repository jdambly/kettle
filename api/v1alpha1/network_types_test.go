package v1alpha1_test

import (
	"github.com/jdambly/kettle/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Network GetAllocatableIPs", func() {
	var (
		network *v1alpha1.Network
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
	})

	Context("when CIDR is provided", func() {
		BeforeEach(func() {
			network.Spec.CIDR = "10.0.1.0/29"
			network.Spec.Gateway = "10.0.1.1"
		})

		It("should generate allocatable IPs based on the CIDR and apply exclusions", func() {
			allocatableIPs, err := network.GetAllocatableIPs()
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
			allocatableIPs, err := network.GetAllocatableIPs()
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
			allocatableIPs, err := network.GetAllocatableIPs()
			Expect(err).To(HaveOccurred())
			Expect(allocatableIPs).To(BeEmpty())
		})
	})
})
