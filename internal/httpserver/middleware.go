package httpserver

import (
	"net/http"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func WithLogger(handler http.Handler, logger logr.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req = req.WithContext(logf.IntoContext(req.Context(), logger))
		handler.ServeHTTP(w, req)
	})
}

func WithLogRequests(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		log := logf.FromContext(req.Context())
		log.Info("Request", "method", req.Method, "url", req.URL.String())
		handler.ServeHTTP(w, req)
	})
}
