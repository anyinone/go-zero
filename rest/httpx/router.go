package httpx

import "net/http"

// Router interface represents a http router that handles http requests.
type Router interface {
	http.Handler
	Handle(string, http.Handler)
	HandleMethod(method, path string, handler http.Handler) error
	SetNotFoundHandler(handler http.Handler)
	SetNotAllowedHandler(handler http.Handler)
}
