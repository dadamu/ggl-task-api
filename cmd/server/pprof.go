package main

import (
	"net/http"
	"net/http/pprof"
)

// newPprofServer returns a server exposing net/http/pprof on its own address and
// mux, so profiling endpoints are never reachable through the API port.
//
// It has no WriteTimeout: /debug/pprof/profile and /trace stream for the
// requested duration (?seconds=N).
func newPprofServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}
}
