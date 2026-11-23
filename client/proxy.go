package client

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// mediaProxy forwards media requests from mpv to APIKeyHost,
// so that the API key never leaves this process.
var mediaProxy struct {
	once   sync.Once
	prefix string
	base   string
	err    error

	stream *http.Client

	errs     map[string]proxyError
	errMutex sync.Mutex
}

// proxyError is an error the media proxy ran into for one target URL.
type proxyError struct {
	err error
	at  time.Time
}

// ErrProxyUnavailable is returned when the media proxy cannot be started.
var ErrProxyUnavailable = errors.New("Client: Media proxy unavailable")

// TakeProxyError returns the most recent unresolved media proxy error and resets all of them.
func TakeProxyError() error {
	mediaProxy.errMutex.Lock()
	defer mediaProxy.errMutex.Unlock()

	var latest proxyError
	for _, entry := range mediaProxy.errs {
		if entry.at.After(latest.at) {
			latest = entry
		}
	}
	mediaProxy.errs = nil

	return latest.err
}

// setProxyError records err for target. A nil err marks target as resolved.
func setProxyError(target string, err error) {
	mediaProxy.errMutex.Lock()
	defer mediaProxy.errMutex.Unlock()

	if err == nil {
		delete(mediaProxy.errs, target)
		return
	}
	if mediaProxy.errs == nil {
		mediaProxy.errs = make(map[string]proxyError)
	}

	mediaProxy.errs[target] = proxyError{err: err, at: time.Now()}
}

var (
	proxyRequestHeaders = []string{
		"Range", "If-Range", "Accept", "Accept-Language",
	}

	proxyResponseHeaders = []string{
		"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges",
		"Last-Modified", "ETag", "Cache-Control", "Content-Disposition",
	}
)

// ProxyURL rewrites uri to the local media proxy if it points to APIKeyHost
// over HTTPS on the default port. Any other URI is returned unchanged.
func ProxyURL(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || !isAPIKeyTarget(u) {
		return uri
	}
	if port := u.Port(); port != "" && port != "443" {
		return uri
	}

	base, err := startProxy()
	if err != nil {
		setProxyError("", fmt.Errorf("%w: %v", ErrProxyUnavailable, err))
		return uri
	}

	proxied := base + u.EscapedPath()
	if u.RawQuery != "" {
		proxied += "?" + u.RawQuery
	}

	return proxied
}

// startProxy starts the media proxy on a random loopback port once,
// and returns its base URL including the secret path prefix.
func startProxy() (string, error) {
	mediaProxy.once.Do(func() {
		secret := make([]byte, 16)
		if _, err := rand.Read(secret); err != nil {
			mediaProxy.err = err
			return
		}

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			mediaProxy.err = err
			return
		}

		mediaProxy.prefix = "/" + hex.EncodeToString(secret)
		mediaProxy.base = "http://" + listener.Addr().String() + mediaProxy.prefix
		mediaProxy.stream = &http.Client{
			Transport:     client.Client.Transport,
			CheckRedirect: checkRedirect,
		}

		server := &http.Server{
			Handler:           http.HandlerFunc(serveProxy),
			ReadHeaderTimeout: 10 * time.Second,
		}
		go server.Serve(listener)
	})

	return mediaProxy.base, mediaProxy.err
}

// serveProxy forwards a GET or HEAD request below the secret prefix to APIKeyHost.
func serveProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, ok := stripProxyPrefix(r.URL.EscapedPath())
	if !ok {
		http.NotFound(w, r)
		return
	}

	target := "https://" + APIKeyHost + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	req, err := newRequest(r.Context(), r.Method, target, nil)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrAPIKeyMissing) {
			status = http.StatusUnauthorized
		}

		setProxyError(target, err)
		http.Error(w, err.Error(), status)
		return
	}

	for _, header := range proxyRequestHeaders {
		if value := r.Header.Get(header); value != "" {
			req.Header.Set(header, value)
		}
	}

	res, err := doWithRateLimit(mediaProxy.stream, req)
	if err != nil {
		err = netError(err)
		if r.Context().Err() == nil {
			setProxyError(target, err)
		}

		var limited *RateLimitError
		if errors.As(err, &limited) {
			w.Header().Set("Retry-After", strconv.Itoa(limited.Seconds()))
			http.Error(w, err.Error(), limited.Status)
			return
		}

		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		// A range past the end is a normal answer while probing or seeking.

	case res.StatusCode >= http.StatusBadRequest:
		if r.Context().Err() == nil {
			setProxyError(target, upstreamError(req, res))
		}

	default:
		setProxyError(target, nil)
	}

	for _, header := range proxyResponseHeaders {
		if value := res.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}

	w.WriteHeader(res.StatusCode)
	if _, err := io.Copy(w, res.Body); err != nil && r.Context().Err() == nil {
		setProxyError(target, netError(err))
	}
}

// upstreamError describes a failed upstream response, using the API key
// error codes where they apply and the answering host otherwise.
func upstreamError(req *http.Request, res *http.Response) error {
	if err := apiKeyResponseError(req, res); err != nil {
		return err
	}

	host := APIKeyHost
	if res.Request != nil {
		host = res.Request.URL.Hostname()
	}

	return fmt.Errorf("%s returned %d", host, res.StatusCode)
}

// stripProxyPrefix removes the secret prefix from path in constant time.
// It reports false if path does not start with the prefix.
func stripProxyPrefix(path string) (string, bool) {
	prefix := mediaProxy.prefix
	if len(path) < len(prefix) {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(path[:len(prefix)]), []byte(prefix)) != 1 {
		return "", false
	}

	rest := path[len(prefix):]
	switch {
	case rest == "":
		return "/", true

	case !strings.HasPrefix(rest, "/"):
		return "", false
	}

	return rest, true
}
