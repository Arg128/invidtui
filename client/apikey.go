package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	// APIKeyHost is the only host that ever receives the API key.
	APIKeyHost = "invidious.nerdvpn.de"

	// APIKeyHeader is the request header that carries the API key.
	APIKeyHeader = "X-Invidious-Token"

	// APIKeyAccessURL is the page where an API key can be requested.
	APIKeyAccessURL = "https://nerdvpn.de/api-access"
)

var (
	// ErrAPIKeyMissing is returned when the API key host is used without an API key.
	ErrAPIKeyMissing = fmt.Errorf("This instance requires an API key, you can get one at: %s", APIKeyAccessURL)

	// ErrAPIKeyNotReceived is returned when the API key host reports a missing key
	// although one was sent.
	ErrAPIKeyNotReceived = fmt.Errorf("API key was not received by %s", APIKeyHost)

	// ErrAccessDenied is returned when the API key host blocks a request.
	ErrAccessDenied = fmt.Errorf("Access denied by %s", APIKeyHost)

	// ErrAPIKeyRejected is returned when the API key host refuses the API key.
	ErrAPIKeyRejected = fmt.Errorf("API key rejected by %s", APIKeyHost)

	errAPIKeyInsecure   = fmt.Errorf("Client: %s is only reachable over HTTPS", APIKeyHost)
	errTooManyRedirects = errors.New("Client: Stopped after 10 redirects")
)

var apiKey struct {
	value string
	mutex sync.RWMutex
}

// SetAPIKey sets the API key for APIKeyHost.
func SetAPIKey(key string) {
	apiKey.mutex.Lock()
	defer apiKey.mutex.Unlock()

	apiKey.value = strings.TrimSpace(key)
}

// APIKey returns the API key for APIKeyHost.
func APIKey() string {
	apiKey.mutex.RLock()
	defer apiKey.mutex.RUnlock()

	return apiKey.value
}

// RequireAPIKey returns ErrAPIKeyMissing if the selected instance is APIKeyHost
// and no API key is set.
func RequireAPIKey() error {
	client.mutex.Lock()
	uri := client.uri
	client.mutex.Unlock()

	return requireAPIKey(uri)
}

// IsAPIKeyInstance reports whether the selected instance is APIKeyHost.
func IsAPIKeyInstance() bool {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	return client.uri != nil && isAPIKeyHost(client.uri)
}

// IsBlockingError reports whether err means that further requests to the host
// fail as well for now: a rate limit, a refused API key or no media proxy.
func IsBlockingError(err error) bool {
	var limited *RateLimitError

	return errors.As(err, &limited) ||
		errors.Is(err, ErrAPIKeyMissing) ||
		errors.Is(err, ErrAPIKeyNotReceived) ||
		errors.Is(err, ErrAPIKeyRejected) ||
		errors.Is(err, ErrAccessDenied) ||
		errors.Is(err, ErrProxyUnavailable)
}

// IsAPIKeyURL reports whether uri points to APIKeyHost.
func IsAPIKeyURL(uri string) bool {
	u, err := url.Parse(uri)

	return err == nil && isAPIKeyHost(u)
}

// requireAPIKey returns an error if uri is APIKeyHost and either no API key
// is set or the scheme is not HTTPS.
func requireAPIKey(uri *url.URL) error {
	if uri == nil || !isAPIKeyHost(uri) {
		return nil
	}

	switch {
	case APIKey() == "":
		return ErrAPIKeyMissing

	case uri.Scheme != "https":
		return errAPIKeyInsecure
	}

	return nil
}

// isAPIKeyHost reports whether u points to APIKeyHost, regardless of scheme.
func isAPIKeyHost(u *url.URL) bool {
	return strings.EqualFold(u.Hostname(), APIKeyHost)
}

// isAPIKeyTarget reports whether u may receive the API key.
func isAPIKeyTarget(u *url.URL) bool {
	return u.Scheme == "https" && isAPIKeyHost(u)
}

// applyAPIKey sets the API key header if req targets APIKeyHost over HTTPS,
// and removes it otherwise.
func applyAPIKey(req *http.Request) error {
	req.Header.Del(APIKeyHeader)

	if !isAPIKeyHost(req.URL) {
		return nil
	}
	if req.URL.Scheme != "https" {
		return errAPIKeyInsecure
	}

	key := APIKey()
	if key == "" {
		return ErrAPIKeyMissing
	}

	req.Header.Set(APIKeyHeader, key)

	return nil
}

// checkRedirect drops the API key from every redirect that leaves APIKeyHost
// or downgrades to plain HTTP.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errTooManyRedirects
	}

	if !isAPIKeyTarget(req.URL) {
		req.Header.Del(APIKeyHeader)
	}

	return nil
}

// apiKeyResponseError maps a 401 or 403 from APIKeyHost to an error, based on
// the "error" field of its JSON body. Responses without a known code are left
// to the caller, with the body intact.
func apiKeyResponseError(req *http.Request, res *http.Response) error {
	if req.Header.Get(APIKeyHeader) == "" {
		return nil
	}
	if res.Request == nil || !isAPIKeyTarget(res.Request.URL) {
		return nil
	}
	if res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden {
		return nil
	}

	data, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	res.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(data), res.Body), res.Body}

	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	json.Unmarshal(data, &body)

	switch body.Error {
	case "credentials_required":
		return ErrAPIKeyNotReceived

	case "invalid_credentials":
		return ErrAPIKeyRejected

	case "access_denied":
		if body.Message != "" {
			return fmt.Errorf("%w: %s", ErrAccessDenied, body.Message)
		}

		return ErrAccessDenied
	}

	return nil
}
