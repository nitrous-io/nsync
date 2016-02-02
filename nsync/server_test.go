package nsync_test

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nsync/nsync"
	"github.com/nitrous-io/nsync/pkg/logger"
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

	newRequest := func(method string, urlStr string, params *url.Values) *http.Request {
		u, err := url.Parse(urlStr)
		Expect(err).NotTo(HaveOccurred())

		if method == "GET" {
			if params != nil {
				u.RawQuery = params.Encode()
			}
			req, err := http.NewRequest(method, u.String(), nil)
			Expect(err).NotTo(HaveOccurred())
			return req
		} else {
			var encodedData io.Reader
			if params != nil {
				encodedData = strings.NewReader(params.Encode())
			}
			req, err := http.NewRequest(method, u.String(), encodedData)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return req
		}

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

			req := newRequest("POST", "/init", data)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "already initialized nsync"}`))
		})
	})

	Describe("POST /containers/{slug}/tunnel/start", func() {
		It("responds with 403 if it is not initialized", func() {
			req = newRequest("POST", "/containers/test-container/tunnel/start", nil)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "token is required"}`))
		})

		It("responds with 403 if arguments are not fulfilled", func() {
			svr.SetSSHInfo("token", "nitrous-path", "ssh-key-path", "key")

			req = newRequest("POST", "/containers/test-container/tunnel/start", nil)

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

			req = newRequest("POST", "/containers/test-container/tunnel/start", data)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "invalid ports"}`))
		})
	})

	Describe("POST /containers/{slug}/sync/start", func() {
		It("responds with 403 if it is not initialized", func() {
			req = newRequest("POST", "/containers/test-container/sync/start", nil)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "token is required"}`))
		})

		It("responds with 403 if arguments are not fulfilled", func() {
			svr.SetSSHInfo("token", "nitrous-path", "ssh-key-path", "key")

			req = newRequest("POST", "/containers/test-container/sync/start", nil)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "arguments are not sufficient"}`))
		})
	})

	Describe("POST /containers/{slug}/sync/stop", func() {
		It("responds with 403 if it is not initialized", func() {
			req = newRequest("POST", "/containers/test-container/sync/stop", nil)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "token is required"}`))
		})
	})

	Describe("POST /containers/{slug}/upload", func() {
		It("responds with 403 if it is not initialized", func() {
			req = newRequest("POST", "/containers/test-container/upload", nil)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "token is required"}`))
		})

		It("responds with 400 if file path is not specified", func() {
			svr.SetSSHInfo("token", "nitrous-path", "ssh-key-path", "key")
			req = newRequest("POST", "/containers/test-container/upload", nil)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "path is required"}`))
		})

		It("responds with 400 if sync is not running", func() {
			svr.SetSSHInfo("token", "nitrous-path", "ssh-key-path", "key")

			data := &url.Values{}
			data.Set("path", "/home/nitrous/.ssh/id_rsa")

			req = newRequest("POST", "/containers/test-container/upload", data)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "sync is not running"}`))
		})
	})

	Describe("POST /containers/{slug}/register_ssh_key", func() {
		It("responds with 403 if it is not initialized", func() {
			req = newRequest("POST", "/containers/test-container/register_ssh_key", nil)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "token is required"}`))
		})
	})

	Describe("PUT /settings", func() {
		var orgLogLevel logger.Level
		BeforeEach(func() {
			orgLogLevel = logger.LogLevel
		})

		AfterEach(func() {
			logger.LogLevel = orgLogLevel
		})

		It("responds with 403 if does not have level parameter", func() {
			req = newRequest("PUT", "/settings", nil)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "arguments are not sufficient"}`))
		})

		It("responds with 403 if does not have valid level parameter", func() {
			data := &url.Values{}
			data.Set("log_level", "whatsup")

			req = newRequest("PUT", "/settings", data)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			message, err := ioutil.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(message)).To(Equal(`{"error": "invalid log level value"}`))
		})

		It("updates log level", func() {
			logger.SetLogLevel("error")

			data := &url.Values{}
			data.Set("log_level", "debug")

			req = newRequest("PUT", "/settings", data)

			handler.ServeHTTP(res, req)
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(logger.LogLevel).To(Equal(logger.DebugLevel))
		})
	})
})
