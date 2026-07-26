# Terminal Protocol Cleanup

Date: 2026-07-26

## Finding

The remaining shim corruption was terminal-emulator state, not kernel termios
state. Codex emits Kitty keyboard-protocol controls and the adapter already
observes `CSI > 7 u`; after an external terminate or force kill, Codex may not
emit its matching cleanup. The local attach client restored `TCSETS`, but that
does not change modes held by the terminal emulator. With Kitty's
disambiguation/event-reporting behavior still active, key events arrive at the
shell as visible CSI-u text such as `s5;1:3u`.

The daemon PTY and the user's terminal are different endpoints:

```text
user terminal emulator <-> harnessctl attach <-> WebSocket <-> daemon PTY <-> Codex
```

Writing cleanup to the PTY master/slave only sends input to Codex. For local
shim recovery, cleanup must be written to the same local terminal output that
rendered the PTY stream (`harnessctl`'s original stdout/controlling terminal).
Termios restoration remains an ioctl on the original local terminal input fd.

## Relevant terminal modes

### Kitty keyboard protocol / CSI-u

Kitty's protocol represents keys as `CSI ... u`. Applications can push flags
with `CSI > flags u`, set flags with `CSI = flags ; mode u`, and pop with
`CSI < number u`. Codex has been observed pushing flags `7`. The specification
requires an application to emit `CSI < u` at exit.

The important recovery detail is that the main and alternate screens maintain
independent keyboard-mode stacks. A killed TUI may be on either screen.
HarnessRelay therefore pops the current screen, leaves alternate screen mode,
then pops the main-screen stack. An empty pop resets the flags, so repeating
the cleanup remains safe. See the
[Kitty keyboard protocol](https://sw.kovidgoyal.net/kitty/keyboard-protocol/).

### xterm modifyOtherKeys

`modifyOtherKeys` is an older xterm mechanism that also changes key encodings
and can use CSI-u-shaped forms. `CSI > 4 ; 0 m` resets its value to disabled.
The current [xterm control-sequence reference](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html)
documents `Pp = 4` as `modifyOtherKeys`.

### Bracketed paste

DEC private mode 2004 wraps pasted bytes in `CSI 200 ~` / `CSI 201 ~`.
`CSI ? 2004 l` disables it. It is not the source of the observed individual
keypress encoding, but leaving it enabled corrupts subsequent shell pastes.

### Mouse tracking and focus events

Modes 1000, 1002, and 1003 enable progressively broader mouse reporting; 1005,
1006, and 1015 select extended coordinate encodings. Mode 1004 enables focus
in/out reports. These modes can inject input bytes into an unsuspecting shell,
so cleanup disables them all. Their DECSET/DECRST behavior is documented in
the [xterm control-sequence reference](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html).

### Alternate screen, cursor, cursor keys, and keypad

Mode 1049 selects the alternate screen while saving/restoring cursor state.
Mode 25 controls cursor visibility. Mode 1 selects application cursor keys,
and `ESC =` / `ESC >` select application/numeric keypad behavior.
HarnessRelay leaves the alternate screen, shows the cursor, disables
application cursor keys, and restores the numeric keypad. A final SGR 0 clears
character attributes without erasing screen content or scrollback.

Codex also uses synchronized output mode 2026 to make redraws atomic. Cleanup
disables it first so a kill between `CSI ? 2026 h` and `CSI ? 2026 l` cannot
leave subsequent shell output buffered or hidden.

## Cleanup sequence

`internal/terminalcleanup.RestoreSequence` emits, in order:

```text
ESC[?2026l              end synchronized output
ESC[<1u                 pop Kitty mode on the active screen
ESC[>4;0m               disable modifyOtherKeys
ESC[?1000l              disable basic mouse tracking
ESC[?1002l              disable button-event mouse tracking
ESC[?1003l              disable any-event mouse tracking
ESC[?1004l              disable focus events
ESC[?1005l              disable UTF-8 mouse coordinates
ESC[?1006l              disable SGR mouse coordinates
ESC[?1015l              disable urxvt mouse coordinates
ESC[?2004l              disable bracketed paste
ESC[?1l                 restore normal cursor keys
ESC>                    restore numeric keypad mode
ESC[?25h                show cursor
ESC[?1049l              leave alternate screen
ESC[<1u                 pop/reset Kitty mode on the main screen
ESC[>4;0m               disable main-screen modifyOtherKeys
ESC[?1l ESC> ESC[?25h   normalize keys/keypad/cursor after screen switch
ESC[0m                  clear character attributes
```

This deliberately avoids RIS (`ESC c`), `reset`, screen erasure, palette
changes, title changes, and other broad visual resets. Those are more
destructive than necessary for routine cleanup.

## Exit-path audit

- Normal child exit: `session.exited` reaches the local WebSocket reader;
  termios and protocol cleanup run before attach returns.
- Web terminate: the session manager records reason `terminate`; the exit event
  triggers cleanup and a remote-termination notice.
- Web force kill: reason `kill` follows the same path; cleanup never relies on
  the child.
- Daemon death / WebSocket EOF: the attach client performs emergency cleanup,
  returns non-zero, and prints recovery/service/bypass instructions.
- Local detach and stdin EOF/error: deferred cleanup runs after the WebSocket is
  closed, preventing later PTY output from re-enabling a mode.
- SIGHUP, SIGINT, SIGQUIT, SIGTERM: the signal watcher restores termios and
  protocol state synchronously before re-raising the signal.
- SIGTSTP: cleanup runs before suspension; raw mode is re-entered after resume,
  and final cleanup remains repeat-safe.
- SIGKILL, kernel failure, or power loss: no local process can run cleanup.
  Manual recovery remains necessary.

## Previous implementation gap

The previous controller called only `rawTerminal.Restore`, which performs
`TCSETS` with the saved `syscall.Termios`. That correctly restored canonical
input, echo, signal generation, and output processing, but it wrote no escape
sequences to the terminal emulator. The local terminal therefore remained in
the protocol state selected by Codex even though `stty` looked normal.

The Codex adapter's parser already confirmed that enhanced keyboard mode is
real: it tracks the latest `CSI > flags u`, `CSI = flags u`, and `CSI < u`
transition to select the correct Enter encoding. Cleanup now addresses the
separate local-emulator boundary.

## Recovery

If an uncatchable failure still leaves a terminal damaged:

```bash
printf '\033[<1u\033[>4;0m\033[?1000;1002;1003;1006l\033[?2004l'
stty sane
reset
```

The first command repairs protocol modes, `stty sane` repairs line discipline,
and `reset` is the final broad fallback.
