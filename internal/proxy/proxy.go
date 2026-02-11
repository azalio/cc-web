package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// TtydProxy creates a reverse proxy handler for a ttyd instance.
func TtydProxy(port int, basePath string) http.Handler {
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal proxy error", http.StatusInternalServerError)
		})
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host

			// Strip our base path prefix if ttyd doesn't use base-path
			// or keep it if ttyd is configured with matching base-path
			if !strings.HasPrefix(req.URL.Path, basePath) {
				req.URL.Path = basePath + req.URL.Path
			}
		},
	}

	return proxy
}
