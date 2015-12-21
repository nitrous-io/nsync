package nitrousapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type NitrousAPI struct {
	token      string
	host       string
	httpClient *http.Client
}

const httpClientTimeout = 10 * time.Second

var ErrContainerNotRunning = errors.New("container is not running")

func NewNitrousAPI(token, host string) *NitrousAPI {
	return &NitrousAPI{token: token, host: host, httpClient: &http.Client{Timeout: httpClientTimeout}}
}

func (na *NitrousAPI) GetUsername() (username string, err error) {
	res, err := na.authedGet("/api/v0/user")
	if err != nil {
		return "", err
	}

	j := map[string]interface{}{}
	json.NewDecoder(res.Body).Decode(&j)

	if user, ok := j["user"].(map[string]interface{}); ok {
		if username, ok := user["username"].(string); ok {
			return username, nil
		}
	}

	return "", errors.New("username was not found")
}

func (na *NitrousAPI) GetContainerInfo(containerSlug string) (hostId int64, sshUrl string, err error) {
	res, err := na.authedGet("/api/v0/containers/" + containerSlug)
	if err != nil {
		return 0, "", err
	}

	j := map[string]interface{}{}
	json.NewDecoder(res.Body).Decode(&j)

	if container, ok := j["container"].(map[string]interface{}); ok {
		if v, ok := container["state"].(string); ok {
			if v != "running" {
				return 0, "", ErrContainerNotRunning
			}
		}

		if v, ok := container["ssh_url"].(string); ok {
			sshUrl = v
		}
		if v, ok := container["host_id"].(float64); ok {
			hostId = int64(v)
		}

		if hostId != 0 && sshUrl != "" {
			return hostId, sshUrl, nil
		}
	}

	return 0, "", errors.New("container info not found")
}

func (na *NitrousAPI) GetHostInfo(hostId int64) (hostSlug string, hostIP string, err error) {
	res, err := na.authedGet(fmt.Sprintf("/api/v0/hosts/%d", hostId))
	if err != nil {
		return "", "", err
	}

	j := map[string]interface{}{}
	json.NewDecoder(res.Body).Decode(&j)

	if host, ok := j["host"].(map[string]interface{}); ok {
		if v, ok := host["slug"].(string); ok {
			hostSlug = v
		}
		if v, ok := host["ip_address"].(string); ok {
			hostIP = v
		}

		if hostSlug != "" && hostIP != "" {
			return hostSlug, hostIP, nil
		}
	}

	return "", "", errors.New("host info not found")
}

// POST /api/v0/containers/<containerslug>/keys  with data - key
func (na *NitrousAPI) UploadPublicKey(containerSlug, pubKey string) error {
	params := map[string]string{
		"key": pubKey,
	}

	res, err := na.authedPost(fmt.Sprintf("/api/v0/containers/%s/keys", containerSlug), params)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusCreated {
		if b, err := ioutil.ReadAll(res.Body); err == nil {
			output := string(b)

			if strings.Contains(output, "key already exists") {
				return nil
			}
		}

		return errors.New("failed to add ssh key")
	}

	return nil
}

func (na *NitrousAPI) authedGet(url string) (res *http.Response, err error) {
	return na.Get(na.host+url, map[string]string{
		"Authorization": "Bearer " + na.token,
	})
}

func (na *NitrousAPI) authedPost(url string, params map[string]string) (res *http.Response, err error) {
	return na.Post(na.host+url, map[string]string{
		"Authorization": "Bearer " + na.token,
	}, params)
}

func (na *NitrousAPI) Get(url string, headers map[string]string) (res *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	return na.httpClient.Do(req)
}

func (na *NitrousAPI) Post(requestUrl string, headers map[string]string, params map[string]string) (res *http.Response, err error) {
	data := url.Values{}
	for k, v := range params {
		data.Set(k, v)
	}

	req, err := http.NewRequest("POST", requestUrl, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Add(k, v)
	}

	return na.httpClient.Do(req)
}
