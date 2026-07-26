package terminalcleanup

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRestoreSequenceContainsProtocolCleanup(t *testing.T) {
	for _, wanted := range []string{
		"\x1b[?2026l",
		"\x1b[<1u",
		"\x1b[>4;0m",
		"\x1b[?2004l",
		"1000",
		"1002",
		"1003",
		"1004",
		"1006",
		"\x1b[?25h",
		"\x1b[?1049l",
	} {
		if !strings.Contains(RestoreSequence, wanted) {
			t.Errorf("RestoreSequence missing %q", wanted)
		}
	}
	if strings.Count(RestoreSequence, "\x1b[<1u") != 2 {
		t.Fatalf("Kitty pop count = %d, want current and main screen pops", strings.Count(RestoreSequence, "\x1b[<1u"))
	}
}

func TestRestoreLocalTerminalIsRepeatSafe(t *testing.T) {
	var out bytes.Buffer
	if err := RestoreLocalTerminal(&out); err != nil {
		t.Fatal(err)
	}
	if err := RestoreLocalTerminal(&out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), RestoreSequence+RestoreSequence; got != want {
		t.Fatalf("repeated cleanup = %q, want %q", got, want)
	}
}

func TestRestoreLocalTerminalToleratesNilAndReportsClosedWriter(t *testing.T) {
	if err := RestoreLocalTerminal(nil); err != nil {
		t.Fatalf("nil writer: %v", err)
	}
	if err := RestoreLocalTerminal(closedWriter{}); !errors.Is(err, errClosedWriter) {
		t.Fatalf("closed writer error = %v", err)
	}
}

func TestRestoreLocalTerminalFlushes(t *testing.T) {
	writer := &flushWriter{}
	if err := RestoreLocalTerminal(writer); err != nil {
		t.Fatal(err)
	}
	if !writer.flushed {
		t.Fatal("cleanup writer was not flushed")
	}
}

var errClosedWriter = errors.New("writer closed")

type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) { return 0, errClosedWriter }

type flushWriter struct {
	bytes.Buffer
	flushed bool
}

func (w *flushWriter) Flush() error {
	w.flushed = true
	return nil
}
