//
// Copyright (c) 2026-present Douglas Hoard
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//

package sshrun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSSHArgOrder(t *testing.T) {
	hc := HostConfig{Hostname: "h1", Port: 2222, Username: "admin", SSHArgs: []string{"-i", "/key.pem", "-o", "StrictHostKeyChecking=no"}}
	got := strings.Join(sshArgs(hc), " ")
	want := "-p 2222 -o BatchMode=yes -o ConnectTimeout=10 -i /key.pem -o StrictHostKeyChecking=no admin@h1"
	if got != want {
		t.Fatalf("sshArgs = %q want %q", got, want)
	}
}

func TestSSHArgNoUser(t *testing.T) {
	hc := HostConfig{Hostname: "h1"}
	got := strings.Join(sshArgs(hc), " ")
	if !strings.HasSuffix(got, " h1") || strings.Contains(got, "@") {
		t.Fatalf("sshArgs no-user = %q", got)
	}
}

func TestSCPArgOrder(t *testing.T) {
	hc := HostConfig{Hostname: "h1", Port: 2222, Username: "admin", SSHArgs: []string{"-i", "/key.pem"}}
	got := strings.Join(scpArgs(hc, "/local/x", "/tmp/r"), " ")
	want := "-P 2222 -o BatchMode=yes -o ConnectTimeout=10 -i /key.pem /local/x admin@h1:/tmp/r"
	if got != want {
		t.Fatalf("scpArgs = %q want %q", got, want)
	}
}

func TestSCPArgNoUser(t *testing.T) {
	hc := HostConfig{Hostname: "h1"}
	got := strings.Join(scpArgs(hc, "/local/x", "/tmp/r"), " ")
	if strings.Contains(got, "@") {
		t.Fatalf("scpArgs no-user should not contain @: %q", got)
	}
	if !strings.HasSuffix(got, "h1:/tmp/r") {
		t.Fatalf("scpArgs no-user = %q", got)
	}
}

func TestShellSingleQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/tmp/fleetsh-abc.tmp", `'/tmp/fleetsh-abc.tmp'`},
		{"plain", `'plain'`},
		{"with space", `'with space'`},
		{"it's", `'it'\''s'`},
		{"", `''`},
	}
	for _, tc := range tests {
		if got := shellSingleQuote(tc.in); got != tc.want {
			t.Errorf("shellSingleQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildRemoteCommandExecutes(t *testing.T) {
	// The generated remote command must run the uploaded script, return its
	// exit status, and delete the script. Execute it through the local POSIX
	// shell to mirror what `ssh host sh -c <command>` does remotely.
	dir := t.TempDir()
	script := filepath.Join(dir, "fleetsh-test.tmp")
	if err := os.WriteFile(script, []byte("echo run-ok; exit 7\n"), 0600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	cmd := exec.Command("sh", "-c", buildRemoteCommand(script))
	out, err := cmd.CombinedOutput()

	if !strings.Contains(string(out), "run-ok") {
		t.Errorf("expected script stdout 'run-ok', got %q", out)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected non-nil ExitError (script exits 7), got err=%v", err)
	}
	if exitErr.ExitCode() != 7 {
		t.Errorf("expected exit code 7 propagated, got %d", exitErr.ExitCode())
	}
	if _, statErr := os.Stat(script); !os.IsNotExist(statErr) {
		t.Errorf("expected uploaded script to be removed, stat err = %v", statErr)
	}
}

func TestBuildRemoteCommandQuotesPathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fleetsh test space.tmp")
	if err := os.WriteFile(script, []byte("echo spaced-ok\n"), 0600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	out, err := exec.Command("sh", "-c", buildRemoteCommand(script)).CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected error running command with spaced path: %v (out=%q)", err, out)
	}
	if !strings.Contains(string(out), "spaced-ok") {
		t.Errorf("expected 'spaced-ok' from spaced-path script, got %q", out)
	}
}

func TestStreamPipesLongLineNotTruncated(t *testing.T) {
	// A single remote line longer than bufio.Scanner's default 64KB limit must
	// be captured intact, not silently truncated.
	const lineLen = 256 * 1024 // 256KB, well over the old 64KB default
	cmd := exec.Command("sh", "-c", "printf 'X%.0s' $(seq 1 "+strconv.Itoa(lineLen)+")")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("StderrPipe: %v", err)
	}

	ch := make(chan StreamEvent, 64)
	go func() {
		streamPipes(cmd, ch, stdout, stderr, "h1", time.Now(), context.Background())
		close(ch)
	}()

	var stdoutLen int
	var done bool
	var scanErr string
	timeout := time.After(30 * time.Second)
collect:
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				break collect
			}
			switch {
			case ev.Done:
				done = true
			case ev.Error != "":
				scanErr = ev.Error
			case !ev.Stderr:
				stdoutLen += len(ev.Line)
			}
		case <-timeout:
			t.Fatal("streamPipes did not complete within timeout (possible truncation/hang)")
		}
	}

	if scanErr != "" {
		t.Fatalf("unexpected scan error: %s", scanErr)
	}
	if !done {
		t.Fatalf("missing Done event")
	}
	if stdoutLen != lineLen {
		t.Errorf("captured stdout length = %d, want %d (line was truncated)", stdoutLen, lineLen)
	}
}
