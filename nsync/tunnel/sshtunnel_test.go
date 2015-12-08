package tunnel_test

import (
	"testing"

	"github.com/nitrous-io/nsync/nsync/tunnel"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "nsync/tunnel")
}

var _ = Describe("nsync/sshtunnel", func() {
	Describe("ParsePort", func() {
		It("parses a port", func() {
			ports, err := tunnel.ParsePort("3000")
			Expect(err).NotTo(HaveOccurred())

			Expect(ports).To(HaveLen(1))
			Expect(ports[0]).To(Equal(3000))
		})

		It("parses ports", func() {
			ports, err := tunnel.ParsePort("3000,5000")
			Expect(err).NotTo(HaveOccurred())

			Expect(ports).To(HaveLen(2))
			Expect(ports).To(ConsistOf(3000, 5000))
		})

		It("works with range", func() {
			ports, err := tunnel.ParsePort("3000-3002,5000")
			Expect(err).NotTo(HaveOccurred())

			Expect(ports).To(HaveLen(4))
			Expect(ports).To(ConsistOf(3000, 3001, 3002, 5000))
		})

		It("returns an error when start port is bigger than end port", func() {
			ports, err := tunnel.ParsePort("3002-3000,5000")
			Expect(err).NotTo(HaveOccurred())

			Expect(ports).To(HaveLen(4))
			Expect(ports).To(ConsistOf(3000, 3001, 3002, 5000))
		})

		It("ignores duplicated ports", func() {
			ports, err := tunnel.ParsePort("3000-3002,3001")
			Expect(err).NotTo(HaveOccurred())

			Expect(ports).To(HaveLen(3))
			Expect(ports).To(ConsistOf(3000, 3001, 3002))
		})

		It("ignores duplicated ports with range", func() {
			ports, err := tunnel.ParsePort("3000-3002,3001-3004")
			Expect(err).NotTo(HaveOccurred())

			Expect(ports).To(HaveLen(5))
			Expect(ports).To(ConsistOf(3000, 3001, 3002, 3003, 3004))
		})

		It("works when first and last port are same", func() {
			ports, err := tunnel.ParsePort("3000-3000")
			Expect(err).NotTo(HaveOccurred())

			Expect(ports).To(HaveLen(1))
			Expect(ports).To(ConsistOf(3000))
		})

		It("returns an error if any ports lower than minium port number", func() {
			ports, err := tunnel.ParsePort("900-3000")
			Expect(err).To(HaveOccurred())
			Expect(ports).To(BeNil())
		})
	})
})
