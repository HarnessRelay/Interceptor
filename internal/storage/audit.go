package storage

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const DefaultAuditLimit = 1024

type AuditRecord struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	Actor     string         `json:"actor"`
	Timestamp time.Time      `json:"ts"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type AuditLog struct {
	mu      sync.Mutex
	limit   int
	records []AuditRecord
}

func NewAuditLog(limit int) *AuditLog {
	if limit <= 0 {
		limit = DefaultAuditLimit
	}
	return &AuditLog{limit: limit}
}

func (l *AuditLog) Record(record AuditRecord) AuditRecord {
	if record.ID == "" {
		record.ID = newAuditID()
	}
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}
	if record.Actor == "" {
		record.Actor = "local"
	}
	if record.Metadata != nil {
		record.Metadata = cloneMetadata(record.Metadata)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
	if len(l.records) > l.limit {
		l.records = l.records[len(l.records)-l.limit:]
	}
	return record
}

func (l *AuditLog) List() []AuditRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]AuditRecord, len(l.records))
	copy(out, l.records)
	for i := range out {
		if out[i].Metadata != nil {
			out[i].Metadata = cloneMetadata(out[i].Metadata)
		}
	}
	return out
}

func cloneMetadata(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func newAuditID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "aud_" + hex.EncodeToString(b)
}
