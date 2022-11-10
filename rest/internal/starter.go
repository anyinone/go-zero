package internal

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/anyinone/go-zero/core/logx"
	"github.com/anyinone/go-zero/core/proc"
)

// StartOption defines the method to customize http.Server.
type StartOption func(svr *http.Server)

// StartHttp starts a http server.
func StartHttp(host string, port int, handler http.Handler, opts ...StartOption) error {
	return start(host, port, handler, func(svr *http.Server) error {
		return svr.ListenAndServe()
	}, opts...)
}

// StartHttps starts a https server.
func StartHttps(host string, port int, certFile, keyFile string, handler http.Handler,
	opts ...StartOption) error {
	return start(host, port, handler, func(svr *http.Server) error {
		// certFile and keyFile are set in buildHttpsServer
		return svr.ListenAndServeTLS(certFile, keyFile)
	}, opts...)
}

func start(host string, port int, handler http.Handler, run func(svr *http.Server) error,
	opts ...StartOption) (err error) {
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", host, port),
		Handler: handler,
	}
	for _, opt := range opts {
		opt(server)
	}

	waitForCalled := proc.AddWrapUpListener(func() {
		if e := server.Shutdown(context.Background()); e != nil {
			logx.Error(e)
		}
	})
	defer func() {
		if err == http.ErrServerClosed {
			waitForCalled()
		}
	}()

	return run(server)
}

// StartHttp starts a http server with listener.
func StartHttpWithListener(ls net.Listener, handler http.Handler, opts ...StartOption) error {
	return startWithListener(ls, handler, func(svr *http.Server) error {
		return svr.Serve(ls)
	}, opts...)
}

// StartHttps starts a https server with listener.
func StartHttpsWithListener(ls net.Listener, certFile, keyFile string, handler http.Handler,
	opts ...StartOption) error {
	return startWithListener(ls, handler, func(svr *http.Server) error {
		// certFile and keyFile are set in buildHttpsServer
		return svr.ServeTLS(ls, certFile, keyFile)
	}, opts...)
}

func startWithListener(ls net.Listener, handler http.Handler, run func(svr *http.Server) error,
	opts ...StartOption) (err error) {
	server := &http.Server{
		Addr:    ls.Addr().String(),
		Handler: handler,
	}
	for _, opt := range opts {
		opt(server)
	}

	waitForCalled := proc.AddWrapUpListener(func() {
		if e := server.Shutdown(context.Background()); e != nil {
			logx.Error(e)
		}
	})
	defer func() {
		if err == http.ErrServerClosed {
			waitForCalled()
		}
	}()

	return run(server)
}
