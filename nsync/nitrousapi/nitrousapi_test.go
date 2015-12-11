package nitrousapi_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nitrous-io/nsync/nsync/nitrousapi"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "nsync/nitrousapi")
}

var _ = Describe("nsync/nitrousapi", func() {
	runServerAndGetClient := func(responseBody string) (*httptest.Server, *http.Client) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"user": {"username":"user1"}}`)
		}))

		transport := &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse(server.URL)
			},
		}

		return server, &http.Client{Transport: transport}
	}

	Describe("GetUsername", func() {
		It("returns username", func() {
			httpServer, httpClient := runServerAndGetClient(`{"user": {"username":"user1"}}`)
			defer httpServer.Close()

			client := nitrousapi.NewNitrousAPI("token", "http://nitrous.test")
			client.SetHttpClient(httpClient)

			// Test the method!
			username, err := client.GetUsername()
			Expect(err).NotTo(HaveOccurred())
			Expect(username).To(Equal("user1"))
		})
	})
})
