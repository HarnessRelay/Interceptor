// Package terminalcleanup restores terminal-emulator protocol state after a
// full-screen application exits without running its own cleanup handlers.
package terminalcleanup

import (
	"errors"
	"io"
)

// RestoreSequence is intentionally narrower than a full terminal reset (RIS).
// It removes protocol modes commonly enabled by TUIs without clearing the
// user's scrollback, palette, title, or other persistent terminal settings.
//
// Kitty keyboard-mode stacks are separate for the main and alternate screens,
// so the sequence pops the current stack, leaves the alternate screen, and
// pops the main-screen stack as well.
const RestoreSequence = "" +
	"\x1b[?2026l" + // End synchronized output so the remaining cleanup is rendered.
	"\x1b[<1u" + // Pop one Kitty keyboard-protocol mode on the current screen.
	"\x1b[>4;0m" + // Disable xterm modifyOtherKeys compatibility mode.
	"\x1b[?1000l" + // Disable basic mouse tracking.
	"\x1b[?1002l" + // Disable button-event mouse tracking.
	"\x1b[?1003l" + // Disable any-event mouse tracking.
	"\x1b[?1004l" + // Disable focus in/out events.
	"\x1b[?1005l" + // Disable UTF-8 extended mouse coordinates.
	"\x1b[?1006l" + // Disable SGR extended mouse coordinates.
	"\x1b[?1015l" + // Disable urxvt extended mouse coordinates.
	"\x1b[?2004l" + // Disable bracketed paste.
	"\x1b[?1l" + // Restore normal (non-application) cursor keys.
	"\x1b>" + // Restore numeric keypad mode.
	"\x1b[?25h" + // Make the cursor visible.
	"\x1b[?1049l" + // Leave the alternate screen, if active.
	"\x1b[<1u" + // Pop/reset the independent main-screen Kitty mode stack.
	"\x1b[>4;0m" + // Also disable modifyOtherKeys on the main screen.
	"\x1b[?1l" +
	"\x1b>" +
	"\x1b[?25h" +
	"\x1b[0m" // Clear character attributes without erasing screen contents.

// RestoreLocalTerminal emits the normal protocol cleanup sequence.
//
// Repeating the sequence is safe: every operation either resets a mode, makes
// a state visible, leaves an inactive alternate screen, or pops an empty Kitty
// stack (which the protocol defines as resetting its flags).
func RestoreLocalTerminal(w io.Writer) error {
	return writeAndFlush(w, RestoreSequence)
}

// EmergencyResetLocalTerminal currently uses the same deliberately bounded
// sequence as normal restoration. It exists as a separate entry point so an
// abnormal-disconnect path can become more defensive later without making
// routine clean exits destructive.
func EmergencyResetLocalTerminal(w io.Writer) error {
	return writeAndFlush(w, RestoreSequence)
}

func writeAndFlush(w io.Writer, sequence string) error {
	if w == nil {
		return nil
	}
	_, writeErr := w.Write([]byte(sequence))
	var flushErr error
	if flusher, ok := w.(interface{ Flush() error }); ok {
		flushErr = flusher.Flush()
	}
	return errors.Join(writeErr, flushErr)
}
