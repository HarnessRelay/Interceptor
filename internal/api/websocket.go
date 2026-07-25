package api

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/security"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func (opts Options) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !isWebSocketRequest(r) {
		writeError(w, http.StatusBadRequest, "websocket upgrade required")
		return
	}
	if !security.SameOrigin(r) {
		writeError(w, http.StatusForbidden, "unexpected origin")
		return
	}
	if _, err := opts.Auth.Authenticate(r); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID != "" {
		if _, ok := opts.Sessions.Get(sessionID); !ok {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
	}
	afterSeq, err := parseUintQuery(r, "after_seq")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing websocket key")
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		writeError(w, http.StatusInternalServerError, "websocket hijack unsupported")
		return
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()

	if _, err := fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", websocketAccept(key)); err != nil {
		return
	}
	if err := rw.Flush(); err != nil {
		return
	}

	for _, event := range opts.Events.History(sessionID, afterSeq, 100) {
		if err := writeWebSocketJSON(conn, event); err != nil {
			return
		}
	}

	sub := opts.Events.Subscribe(events.SubscribeOptions{
		SessionID: sessionID,
		Buffer:    128,
	})
	defer sub.Close()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-sub.C:
			if !ok {
				return
			}
			if afterSeq > 0 && event.SessionID == sessionID && event.Sequence <= afterSeq {
				continue
			}
			if err := writeWebSocketJSON(conn, event); err != nil {
				return
			}
		}
	}
}

func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		headerContainsToken(r.Header.Get("Connection"), "upgrade") &&
		r.Header.Get("Sec-WebSocket-Version") == "13"
}

func headerContainsToken(value, token string) bool {
	for _, part := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func writeWebSocketJSON(conn net.Conn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return writeWebSocketFrame(conn, 0x1, data)
}

func writeWebSocketFrame(conn net.Conn, opcode byte, payload []byte) error {
	header := []byte{0x80 | opcode}
	switch {
	case len(payload) <= 125:
		header = append(header, byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 126, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(len(payload)))
	default:
		header = append(header, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[len(header)-8:], uint64(len(payload)))
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}
