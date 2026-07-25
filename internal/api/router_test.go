package api

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHealthEndpoint(t *testing.T) {
	router := NewRouter(Options{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:  "test-version",
		StaticFS: testStaticFS(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	want := `{"status":"ok","service":"harnessd","version":"test-version"}`
	if strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("body = %q, want %q", strings.TrimSpace(rec.Body.String()), want)
	}
}

func TestStaticRootAndAPINotFoundAreSeparated(t *testing.T) {
	router := NewRouter(Options{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:  "test-version",
		StaticFS: testStaticFS(),
	})

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRec := httptest.NewRecorder()
	router.ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d", rootRec.Code, http.StatusOK)
	}
	if !strings.Contains(rootRec.Body.String(), "HarnessRelay Interceptor") {
		t.Fatalf("root body did not contain dashboard placeholder: %q", rootRec.Body.String())
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	apiRec := httptest.NewRecorder()
	router.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusNotFound {
		t.Fatalf("api miss status = %d, want %d", apiRec.Code, http.StatusNotFound)
	}
	if strings.Contains(apiRec.Body.String(), "HarnessRelay Interceptor") {
		t.Fatalf("api miss served static dashboard: %q", apiRec.Body.String())
	}
}

func testStaticFS() fs.FS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>HarnessRelay Interceptor</title>")},
	}
}
