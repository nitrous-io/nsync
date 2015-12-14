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
	var client *nitrousapi.NitrousAPI

	runServerAndGetClient := func(responseBody string, responseCode int) (*httptest.Server, *http.Client) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(responseCode)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, responseBody)
		}))

		transport := &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse(server.URL)
			},
		}

		return server, &http.Client{Transport: transport}
	}

	BeforeEach(func() {
		client = nitrousapi.NewNitrousAPI("token", "http://nitrous.test")
	})

	Describe("GetUsername", func() {
		It("returns username", func() {
			httpServer, httpClient := runServerAndGetClient(`{"user": {"username":"user1"}}`, http.StatusOK)
			defer httpServer.Close()

			client.SetHttpClient(httpClient)

			username, err := client.GetUsername()
			Expect(err).NotTo(HaveOccurred())
			Expect(username).To(Equal("user1"))
		})
	})

	Describe("GetContainerInfo", func() {
		It("returns host ID and ssh url", func() {
			httpServer, httpClient := runServerAndGetClient(`{"container": {"ssh_url":"ssh://nitrous@nitrous.io", "host_id": 1234}}`, http.StatusOK)
			defer httpServer.Close()

			client.SetHttpClient(httpClient)

			hostID, sshUrl, err := client.GetContainerInfo("container-slug")
			Expect(err).NotTo(HaveOccurred())
			Expect(hostID).To(Equal(int64(1234)))
			Expect(sshUrl).To(Equal("ssh://nitrous@nitrous.io"))
		})
	})

	Describe("GetHostInfo", func() {
		It("returns host ID and host ip", func() {
			httpServer, httpClient := runServerAndGetClient(`{"host": {"slug":"host-slug", "ip_address": "1.2.3.4"}}`, http.StatusOK)
			defer httpServer.Close()

			client.SetHttpClient(httpClient)

			hostSlug, hostIP, err := client.GetHostInfo(123)
			Expect(err).NotTo(HaveOccurred())
			Expect(hostSlug).To(Equal("host-slug"))
			Expect(hostIP).To(Equal("1.2.3.4"))
		})
	})

	Describe("UploadPublicKey", func() {
		It("does not return an error", func() {
			httpServer, httpClient := runServerAndGetClient(`{"status":"created"}`, http.StatusCreated)
			defer httpServer.Close()

			client.SetHttpClient(httpClient)

			err := client.UploadPublicKey("container-slug", "public-key")
			Expect(err).NotTo(HaveOccurred())
		})

		It("does not return an error when response message contains 'key already exists'", func() {
			httpServer, httpClient := runServerAndGetClient(`{"error_description":"key already exists"}}`, http.StatusForbidden)
			defer httpServer.Close()

			client.SetHttpClient(httpClient)

			err := client.UploadPublicKey("container-slug", "public-key")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
