package nsync_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nsync/nsync"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "nsync")
}

var _ = Describe("nsync/server", func() {
	var (
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		err     error
		svr     *nsync.Server
	)

	BeforeEach(func() {
		// func NewServer(token, sshKeyPath, nitrousFolderPath, pubKey string, stopCh chan struct{})
		svr = nsync.NewServer("token", "", "", "", nil)
		handler = svr.Router()
		res = httptest.NewRecorder()
	})

	Describe("GET /ping", func() {
		It("responds with 200", func() {
			req, err = http.NewRequest("GET", "/ping", nil)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("POST /containers/{slug}/tunnel/start", func() {
		It("responds with 403 if arguments are not fulfilled", func() {
			req, err = http.NewRequest("POST", "/containers/test-container/tunnel/start", nil)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))
		})

		It("responds with 403 if ports are not valid", func() {
			data := url.Values{}
			data.Set("sshUrl", "ssh://nitrous@pete-4644.nitrouspro.com:32866")
			data.Set("ports", ",,")
			req, err = http.NewRequest("POST", "/containers/test-container/tunnel/start", nil)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))
		})
	})

	Describe("GET /subscribe", func() {
		It("responds with 200", func() {
			req, err = http.NewRequest("GET", "/subscribe", nil)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})
})
