package event

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "nsync/event")
}

var _ = Describe("nsync/event", func() {
	Describe("Send", func() {
		It("sends a message to channel", func() {
			e := NewEvent(10)

			e.Send("event-name", map[string]string{
				"hello": "world",
			})

			select {
			case b := <-e.MsgCh:
				var message Message
				err := json.Unmarshal(b, &message)
				Expect(err).NotTo(HaveOccurred())
				Expect(message.Name).To(Equal("event-name"))
				data := message.Data.(map[string]interface{})
				Expect(data["hello"].(string)).To(Equal("world"))
			case <-time.After(1 * time.Second):
				Fail("timed out waiting for message")
			}
		})
	})
})
