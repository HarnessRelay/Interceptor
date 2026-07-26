package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type recordedCommand struct {
	name string
	args []string
}

type fakeRunner struct {
	commands []recordedCommand
	failAt   int
}

func (r *fakeRunner) Run(_ context.Context, name string, args []string, stdout, _ io.Writer) error {
	r.commands = append(r.commands, recordedCommand{name: name, args: append([]string(nil), args...)})
	if r.failAt > 0 && len(r.commands) == r.failAt {
		return errors.New("fake failure")
	}
	_, _ = io.WriteString(stdout, "fake output\n")
	return nil
}

func executableFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestUnitUsesAbsoluteQuotedDaemonAndSafePolicy(t *testing.T) {
	dir := t.TempDir()
	daemon := filepath.Join(dir, "bin with spaces", "harnessd")
	if err := os.MkdirAll(filepath.Dir(daemon), 0o755); err != nil {
		t.Fatal(err)
	}
	executableFixture(t, daemon)

	unit, err := Unit(daemon)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{
		managedMarker,
		`ExecStart="` + daemon + `" serve`,
		"Type=exec",
		"Restart=on-failure",
		"WantedBy=default.target",
	} {
		if !strings.Contains(unit, wanted) {
			t.Fatalf("unit missing %q:\n%s", wanted, unit)
		}
	}
	if strings.Contains(unit, "0.0.0.0") || strings.Contains(unit, "HARNESSRELAY_TOKEN=") {
		t.Fatalf("unit must preserve config security defaults:\n%s", unit)
	}
}

func TestInstallWritesOwnedUnitAndReloadsOnly(t *testing.T) {
	dir := t.TempDir()
	daemon := filepath.Join(dir, "harnessd")
	executableFixture(t, daemon)
	runner := &fakeRunner{}
	manager := &Manager{
		UnitPath:   filepath.Join(dir, "config", "systemd", "user", UnitName),
		DaemonPath: daemon, Systemctl: "systemctl", Journalctl: "journalctl", Runner: runner,
	}
	var stdout bytes.Buffer
	if err := manager.Install(context.Background(), &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(manager.UnitPath)
	if err != nil {
		t.Fatal(err)
	}
	if !IsManaged(content) {
		t.Fatal("installed unit is not ownership marked")
	}
	if got, want := runner.commands, []recordedCommand{{name: "systemctl", args: []string{"--user", "daemon-reload"}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestInstallRefusesUnmanagedUnit(t *testing.T) {
	dir := t.TempDir()
	daemon := filepath.Join(dir, "harnessd")
	executableFixture(t, daemon)
	unitPath := filepath.Join(dir, UnitName)
	if err := os.WriteFile(unitPath, []byte("[Service]\nExecStart=/something-else\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	manager := &Manager{UnitPath: unitPath, DaemonPath: daemon, Systemctl: "systemctl", Runner: runner}
	if err := manager.Install(context.Background(), io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("Install error = %v, want unmanaged refusal", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("unexpected commands: %#v", runner.commands)
	}
}

func TestUninstallPreservesUnmanagedAndStopsOwnedBeforeRemoval(t *testing.T) {
	dir := t.TempDir()
	daemon := filepath.Join(dir, "harnessd")
	executableFixture(t, daemon)
	unitPath := filepath.Join(dir, UnitName)
	manager := &Manager{UnitPath: unitPath, DaemonPath: daemon, Systemctl: "systemctl", Runner: &fakeRunner{}}
	if err := os.WriteFile(unitPath, []byte("unmanaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := manager.Uninstall(context.Background(), io.Discard, io.Discard); err == nil {
		t.Fatal("expected unmanaged unit refusal")
	}
	if _, err := os.Stat(unitPath); err != nil {
		t.Fatalf("unmanaged unit was removed: %v", err)
	}

	if err := os.WriteFile(unitPath, []byte(managedMarker+"\n[Service]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	manager.Runner = runner
	if err := manager.Uninstall(context.Background(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unitPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned unit still exists: %v", err)
	}
	want := []recordedCommand{
		{name: "systemctl", args: []string{"--user", "disable", "--now", UnitName}},
		{name: "systemctl", args: []string{"--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestUninstallPreservesOwnedUnitWhenDisableFails(t *testing.T) {
	dir := t.TempDir()
	unitPath := filepath.Join(dir, UnitName)
	if err := os.WriteFile(unitPath, []byte(managedMarker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{UnitPath: unitPath, Systemctl: "systemctl", Runner: &fakeRunner{failAt: 1}}
	if err := manager.Uninstall(context.Background(), io.Discard, io.Discard); err == nil {
		t.Fatal("expected disable failure")
	}
	if _, err := os.Stat(unitPath); err != nil {
		t.Fatalf("owned unit should be preserved: %v", err)
	}
}

func TestLifecycleCommandsUseUserManagerAndJournal(t *testing.T) {
	runner := &fakeRunner{}
	manager := &Manager{Systemctl: "systemctl", Journalctl: "journalctl", Runner: runner}
	ctx := context.Background()
	for _, call := range []func() error{
		func() error { return manager.Start(ctx, io.Discard, io.Discard) },
		func() error { return manager.Stop(ctx, io.Discard, io.Discard) },
		func() error { return manager.Restart(ctx, io.Discard, io.Discard) },
		func() error { return manager.Enable(ctx, io.Discard, io.Discard) },
		func() error { return manager.Disable(ctx, io.Discard, io.Discard) },
		func() error { return manager.Status(ctx, io.Discard, io.Discard) },
		func() error { return manager.Logs(ctx, io.Discard, io.Discard) },
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	want := []recordedCommand{
		{name: "systemctl", args: []string{"--user", "start", UnitName}},
		{name: "systemctl", args: []string{"--user", "stop", UnitName}},
		{name: "systemctl", args: []string{"--user", "restart", UnitName}},
		{name: "systemctl", args: []string{"--user", "enable", UnitName}},
		{name: "systemctl", args: []string{"--user", "disable", UnitName}},
		{name: "systemctl", args: []string{"--user", "status", "--no-pager", UnitName}},
		{name: "journalctl", args: []string{"--user", "--unit", UnitName, "--no-pager"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}
