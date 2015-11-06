package nsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
)

var nitrousHost = os.Getenv("NITROUS_HOST")

func init() {
	if nitrousHost == "" {
		nitrousHost = "https://www.nitrous.io"
	}
}

func (ns *Nsync) getUsername() (username string, err error) {
	res, err := ns.authedGet("/api/v0/user")
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

func (ns *Nsync) getContainerInfo(containerSlug string) (hostId int64, sshUrl string, err error) {
	res, err := ns.authedGet("/api/v0/containers/" + containerSlug)
	if err != nil {
		return 0, "", err
	}

	j := map[string]interface{}{}
	json.NewDecoder(res.Body).Decode(&j)

	if container, ok := j["container"].(map[string]interface{}); ok {
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

func (ns *Nsync) getHostInfo(hostId int64) (hostSlug string, hostIP string, err error) {
	res, err := ns.authedGet(fmt.Sprintf("/api/v0/hosts/%d", hostId))
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

func (ns *Nsync) uploadPublicKey(pubKey string) error {
	// POST /api/v0/containers/<containerslug>/keys  with data - key
	return nil
}

func (ns *Nsync) authedGet(url string) (res *http.Response, err error) {
	return Get(url, map[string]string{
		"Authorization": "Bearer " + ns.token,
	})
}

func Get(url string, headers map[string]string) (res *http.Response, err error) {
	req, err := http.NewRequest("GET", nitrousHost+url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	return http.DefaultClient.Do(req)
}
