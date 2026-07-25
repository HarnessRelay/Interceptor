package logging

import (
	"io"
	"log/slog"
	"os"
)

const (
	RequestIDKey = "request_id"
	SessionIDKey = "session_id"
)

func New(w io.Writer, level slog.Leveler) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

func RequestID(id string) slog.Attr {
	return slog.String(RequestIDKey, id)
}

func SessionID(id string) slog.Attr {
	return slog.String(SessionIDKey, id)
}
