package httpx

import (
	"net/http"
	"net/url"
)

// Router interface represents a http router that handles http requests.
type Router interface {
	http.Handler
	Handle(string, http.Handler)
	HandleHub(string, http.Handler)
	Proxy(match string, target *url.URL)
	HandleMethod(method, path string, handler http.Handler) error
	SetNotFoundHandler(handler http.Handler)
	SetNotAllowedHandler(handler http.Handler)
}
