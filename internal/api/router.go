package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/harnessrelay/interceptor/internal/logging"
)

type Options struct {
	Logger   *slog.Logger
	Version  string
	StaticFS fs.FS
}

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

type errorResponse struct {
	Error string `json:"error"`
}

var requestCounter uint64

func NewRouter(opts Options) http.Handler {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.StaticFS == nil {
		opts.StaticFS = fs.FS(osDirFallback{})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{
			Status:  "ok",
			Service: "harnessd",
			Version: opts.Version,
		})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.Handle("/", http.FileServerFS(opts.StaticFS))

	return requestLogMiddleware(opts.Logger, mux)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func requestLogMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = nextRequestID()
		}
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("http request",
			logging.RequestID(requestID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Duration("duration", time.Since(started)),
		)
	})
}

func nextRequestID() string {
	return fmt.Sprintf("req-%d", atomic.AddUint64(&requestCounter, 1))
}

type osDirFallback struct{}

func (osDirFallback) Open(name string) (fs.File, error) {
	return nil, fs.ErrNotExist
}
