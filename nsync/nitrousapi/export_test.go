package nitrousapi

import "net/http"

func (na *NitrousAPI) SetHttpClient(client *http.Client) {
	na.httpClient = client
}
