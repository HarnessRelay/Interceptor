package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatusCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "harnessd",
			"version": "test",
		})
	}))
	defer server.Close()
	t.Setenv("HARNESSRELAY_ADDR", server.URL)

	var out bytes.Buffer
	if err := run([]string{"status"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run status: %v", err)
	}
	if !strings.Contains(out.String(), "harnessd ok (test)") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestReadWebSocketFrame(t *testing.T) {
	var frame bytes.Buffer
	payload := []byte(`{"type":"terminal.output"}`)
	frame.WriteByte(0x81)
	frame.WriteByte(byte(len(payload)))
	frame.Write(payload)

	got, err := readWebSocketFrame(bufio.NewReader(&frame))
	if err != nil {
		t.Fatalf("readWebSocketFrame: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q, want %q", string(got), string(payload))
	}
}

func TestReadWebSocketFrameExtendedLength(t *testing.T) {
	var frame bytes.Buffer
	payload := bytes.Repeat([]byte("x"), 130)
	frame.WriteByte(0x81)
	frame.WriteByte(126)
	var size [2]byte
	binary.BigEndian.PutUint16(size[:], uint16(len(payload)))
	frame.Write(size[:])
	frame.Write(payload)

	got, err := readWebSocketFrame(bufio.NewReader(&frame))
	if err != nil {
		t.Fatalf("readWebSocketFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload length = %d, want %d", len(got), len(payload))
	}
}

func TestStreamAttachInputDetach(t *testing.T) {
	c := client{}
	err := c.streamAttachInput(bytes.NewBuffer([]byte{0x1d}), "ses_test")
	if !errors.Is(err, errDetach) {
		t.Fatalf("streamAttachInput error = %v, want detach", err)
	}
}

func TestRunCommandUsesBearerTokenAndPayload(t *testing.T) {
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sessions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization") == "Bearer secret"
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["name"] != "demo" || body["cwd"] != "/tmp/project" || body["command"] != "/bin/bash" {
			t.Fatalf("unexpected body: %+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session": map[string]any{
				"id":      "ses_test",
				"status":  "running",
				"command": "/bin/bash",
			},
		})
	}))
	defer server.Close()
	t.Setenv("HARNESSRELAY_ADDR", server.URL)
	t.Setenv("HARNESSRELAY_TOKEN", "secret")

	var out bytes.Buffer
	err := run([]string{"run", "--name", "demo", "--cwd", "/tmp/project", "/bin/bash", "-l"}, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if !sawAuth {
		t.Fatal("missing bearer auth")
	}
	if !strings.Contains(out.String(), "ses_test") {
		t.Fatalf("output = %q", out.String())
	}
}
