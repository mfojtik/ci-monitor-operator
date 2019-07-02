package gitserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"k8s.io/klog"
)

type key int

const (
	requestIDKey key = 0
)

var (
	listenAddr string
	healthy    int32
	repoDir    string
)

func Run(directory string, addr string) {
	listenAddr = addr
	repoDir = directory

	klog.Infof("Started serving GIT repository on %s/cluster-config ...", listenAddr)

	router := http.NewServeMux()
	router.Handle("/cluster-config", index())
	router.Handle("/cluster-config/healthz", healthz())

	nextRequestID := func() string {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      tracing(nextRequestID)(logging()(router)),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		klog.Infof("Git HTTP Server is shutting down...")
		atomic.StoreInt32(&healthy, 0)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			klog.Fatalf("Could not gracefully shutdown the server: %v\n", err)
		}
		close(done)
	}()

	klog.Infof("Server is ready to handle requests at", listenAddr)
	atomic.StoreInt32(&healthy, 1)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		klog.Fatalf("Could not listen on %s: %v\n", listenAddr, err)
	}

	<-done
}

type prefixedHandler struct {
	prefix string
}

func (h *prefixedHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	fileServer := http.FileServer(http.Dir(filepath.Join(repoDir, ".git")))
	req.URL.Path = strings.TrimPrefix(req.URL.Path, h.prefix)
	fileServer.ServeHTTP(w, req)
}

func index() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := &prefixedHandler{
			prefix: "/cluster-config",
		}
		handler.ServeHTTP(w, r)
	})
}

func healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&healthy) == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}

func logging() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				requestID, ok := r.Context().Value(requestIDKey).(string)
				if !ok {
					requestID = "unknown"
				}
				klog.Infof("[%s][%s] %q [%s](%s)", requestID, r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func tracing(nextRequestID func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-Id")
			if requestID == "" {
				requestID = nextRequestID()
			}
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			w.Header().Set("X-Request-Id", requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
