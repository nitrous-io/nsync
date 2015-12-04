package nsync_test

import (
	"io/ioutil"
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

	newRequest := func(method string, urlStr string, params *url.Values) (*http.Request, error) {
		u, err := url.Parse(urlStr)
		if err != nil {
			return nil, err
		}
		if params != nil {
			u.RawQuery = params.Encode()
		}
		return http.NewRequest(method, u.String(), nil)
	}

	BeforeEach(func() {
		svr = nsync.NewServer(nil)
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

	Describe("POST /init", func() {
		It("responds with 403 if arguments are not fulfilled", func() {
			req, err = http.NewRequest("POST", "/init", nil)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "token is required"}`))
		})

		It("responds with 403 if it is already initialized", func() {
			svr.SetSSHInfo("token", "nitrous-path", "ssh-key-path", "key")

			data := &url.Values{}
			data.Set("token", "token")
			data.Set("nitrousPath", "nitrousPath")

			req, err := newRequest("POST", "/init", data)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "already initialized nsync"}`))
		})
	})

	Describe("POST /containers/{slug}/tunnel/start", func() {
		It("responds with 403 if it is not initialized", func() {
			req, err = newRequest("POST", "/containers/test-container/tunnel/start", nil)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "token is required"}`))
		})

		It("responds with 403 if arguments are not fulfilled", func() {
			svr.SetSSHInfo("token", "nitrous-path", "ssh-key-path", "key")

			req, err = newRequest("POST", "/containers/test-container/tunnel/start", nil)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "arguments are not sufficient"}`))
		})

		It("responds with 403 if ports are not valid", func() {
			svr.SetSSHInfo("token", "nitrous-path", "ssh-key-path", "key")

			data := &url.Values{}
			data.Set("ports", ",,")

			req, err = newRequest("POST", "/containers/test-container/tunnel/start", data)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "invalid ports"}`))
		})
	})

	Describe("POST /containers/{slug}/sync/start", func() {
		It("responds with 403 if it is not initialized", func() {
			req, err = newRequest("POST", "/containers/test-container/sync/start", nil)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "token is required"}`))
		})

		It("responds with 403 if arguments are not fulfilled", func() {
			svr.SetSSHInfo("token", "nitrous-path", "ssh-key-path", "key")

			req, err = newRequest("POST", "/containers/test-container/sync/start", nil)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "arguments are not sufficient"}`))
		})
	})

	Describe("POST /containers/{slug}/sync/stop", func() {
		It("responds with 403 if it is not initialized", func() {
			req, err = newRequest("POST", "/containers/test-container/sync/stop", nil)
			Expect(err).To(BeNil())

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "token is required"}`))
		})
	})
})
