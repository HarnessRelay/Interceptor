package storage

import "testing"

func TestAuditLogIsBounded(t *testing.T) {
	log := NewAuditLog(2)
	log.Record(AuditRecord{Type: "one"})
	log.Record(AuditRecord{Type: "two"})
	log.Record(AuditRecord{Type: "three"})

	records := log.List()
	if len(records) != 2 {
		t.Fatalf("records length = %d, want 2", len(records))
	}
	if records[0].Type != "two" || records[1].Type != "three" {
		t.Fatalf("unexpected records: %+v", records)
	}
}

func TestAuditLogCopiesMetadata(t *testing.T) {
	log := NewAuditLog(1)
	metadata := map[string]any{"bytes": 10}
	record := log.Record(AuditRecord{Type: "input", Metadata: metadata})
	metadata["bytes"] = 99

	if record.Metadata["bytes"] != 10 {
		t.Fatalf("record metadata changed: %+v", record.Metadata)
	}
	records := log.List()
	records[0].Metadata["bytes"] = 42
	if log.List()[0].Metadata["bytes"] != 10 {
		t.Fatal("List returned mutable internal metadata")
	}
}
