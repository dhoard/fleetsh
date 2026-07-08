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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorReader returns data from data []byte with nil error on the first Read,
// then returns err on subsequent Reads.
type errorReader struct {
	data []byte
	err  error
	done bool
}

func (r *errorReader) Read(p []byte) (int, error) {
	if !r.done {
		r.done = true
		if len(r.data) > 0 {
			n := copy(p, r.data)
			return n, nil
		}
	}
	return 0, r.err
}

// chunkedErrorReader returns successive chunks from its slice, then an error.
// If a chunk is larger than the caller's buffer, multiple Reads are required
type chunkedErrorReader struct {
	chunks [][]byte
	err    error
	pos    int
	offset int // offset within current chunk
}

func (r *chunkedErrorReader) Read(p []byte) (int, error) {
	for r.pos < len(r.chunks) {
		chunk := r.chunks[r.pos]
		if r.offset >= len(chunk) {
			r.pos++
			r.offset = 0
			continue
		}
		n := copy(p, chunk[r.offset:])
		r.offset += n
		return n, nil
	}
	return 0, r.err
}

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

func TestSSHArgs_TTY(t *testing.T) {
	// TTY=true: first element must be "-tt"
	hcTrue := HostConfig{Hostname: "h1", Username: "admin", TTY: true}
	argsTrue := sshArgs(hcTrue)
	require.NotEmpty(t, argsTrue)
	assert.Equal(t, "-tt", argsTrue[0], "-tt must be first argument when TTY=true")

	// TTY=false: no "-tt" anywhere
	hcFalse := HostConfig{Hostname: "h1", Username: "admin", TTY: false}
	argsFalse := sshArgs(hcFalse)
	assert.NotContains(t, argsFalse, "-tt", "-tt must not appear when TTY=false")

	// Zero value: no "-tt" anywhere
	hcZero := HostConfig{Hostname: "h1", Username: "admin"}
	argsZero := sshArgs(hcZero)
	assert.NotContains(t, argsZero, "-tt", "-tt must not appear when TTY is zero value")
}

func TestSSHArgs_TTYWithPort(t *testing.T) {
	hc := HostConfig{Hostname: "h1", Port: 2222, Username: "admin", TTY: true}
	args := sshArgs(hc)
	require.NotEmpty(t, args)
	assert.Equal(t, "-tt", args[0], "-tt must be first argument")

	// -tt must precede -p
	ttIdx := -1
	portIdx := -1
	for i, a := range args {
		if a == "-tt" {
			ttIdx = i
		}
		if a == "-p" {
			portIdx = i
		}
	}
	require.NotEqual(t, -1, ttIdx, "-tt must be present")
	require.NotEqual(t, -1, portIdx, "-p must be present")
	assert.Less(t, ttIdx, portIdx, "-tt must precede -p")
}

func TestSSHArgs_TTYWithSSHArgs(t *testing.T) {
	hc := HostConfig{Hostname: "h1", Username: "admin", TTY: true, SSHArgs: []string{"-i", "/key.pem"}}
	args := sshArgs(hc)
	require.NotEmpty(t, args)
	assert.Equal(t, "-tt", args[0], "-tt must be first argument")

	// -tt must precede SSH args
	ttIdx := -1
	keyIdx := -1
	for i, a := range args {
		if a == "-tt" {
			ttIdx = i
		}
		if a == "-i" {
			keyIdx = i
		}
	}
	require.NotEqual(t, -1, ttIdx, "-tt must be present")
	require.NotEqual(t, -1, keyIdx, "-i must be present")
	assert.Less(t, ttIdx, keyIdx, "-tt must precede -i")
}

func TestSCPArgs_NoTTY(t *testing.T) {
	// scpArgs must never include -tt even when TTY is true
	hc := HostConfig{Hostname: "h1", Username: "admin", TTY: true}
	args := scpArgs(hc, "/local/x", "/tmp/r")
	assert.NotContains(t, args, "-tt", "scpArgs must never include -tt")
}

func TestEmitLine_StripsCarriageReturn(t *testing.T) {
	ch := make(chan StreamEvent, 8)

	// Case 1: trailing \r is stripped
	emitLine(ch, "h1", false, []byte("hello world\r"), false)
	require.Len(t, ch, 1)
	ev := <-ch
	assert.Equal(t, "hello world", ev.Line)
	assert.False(t, ev.Stderr)

	// Case 2: no \r → unchanged
	emitLine(ch, "h1", false, []byte("no cr"), false)
	require.Len(t, ch, 1)
	ev = <-ch
	assert.Equal(t, "no cr", ev.Line)

	// Case 3: embedded \r\n preserved, only trailing \r stripped
	emitLine(ch, "h1", false, []byte("mixed\r\nstill\r"), false)
	require.Len(t, ch, 1)
	ev = <-ch
	assert.Equal(t, "mixed\r\nstill", ev.Line)

	// Case 4: only \r → empty after strip → no event emitted
	emitLine(ch, "h1", false, []byte("\r"), false)
	assert.Len(t, ch, 0, "no event should be emitted for bare \\r")

	// Case 5: truncated + trailing \r → stripped before marker
	emitLine(ch, "h1", false, []byte("truncated line\r"), true)
	require.Len(t, ch, 1)
	ev = <-ch
	assert.Equal(t, "truncated line (truncated)", ev.Line)
}

func TestStreamReader_StripsTrailingCR(t *testing.T) {
	input := "line1\r\nline2\r\nline3\r\n"
	ch := make(chan StreamEvent, 8)

	go func() {
		streamReader(strings.NewReader(input), ch, "h1", false, false)
		close(ch)
	}()

	events := drainChannel(ch)
	require.Len(t, events, 3)
	assert.Equal(t, "line1", events[0].Line)
	assert.Equal(t, "line2", events[1].Line)
	assert.Equal(t, "line3", events[2].Line)
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
		streamPipes(cmd, ch, stdout, stderr, "h1", time.Now(), context.Background(), false)
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

func TestStreamPipesNoTrunc(t *testing.T) {
	const lineLen = 2 * 1024 * 1024
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
		streamPipes(cmd, ch, stdout, stderr, "h1", time.Now(), context.Background(), true)
		close(ch)
	}()

	var stdoutLen int
	var done bool
	var scanErr string
	var truncatedMarkerSeen bool
	timeout := time.After(60 * time.Second)
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
				if strings.Contains(ev.Line, "(truncated)") {
					truncatedMarkerSeen = true
				}
			}
		case <-timeout:
			t.Fatal("streamPipes did not complete within timeout")
		}
	}

	if scanErr != "" {
		t.Fatalf("unexpected scan error: %s", scanErr)
	}
	if !done {
		t.Fatalf("missing Done event")
	}
	if truncatedMarkerSeen {
		t.Errorf("unexpected (truncated) marker when noTrunc=true")
	}
	if stdoutLen != lineLen {
		t.Errorf("captured stdout length = %d, want %d (line was truncated)", stdoutLen, lineLen)
	}
}

func TestStreamPipesTruncatedDefault(t *testing.T) {
	const lineLen = 2 * 1024 * 1024
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
		streamPipes(cmd, ch, stdout, stderr, "h1", time.Now(), context.Background(), false)
		close(ch)
	}()

	var stdoutLen int
	var done bool
	var scanErr string
	var truncatedMarkerSeen bool
	timeout := time.After(60 * time.Second)
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
				if strings.Contains(ev.Line, "(truncated)") {
					truncatedMarkerSeen = true
				}
			}
		case <-timeout:
			t.Fatal("streamPipes did not complete within timeout")
		}
	}

	if scanErr != "" {
		t.Fatalf("unexpected scan error: %s", scanErr)
	}
	if !done {
		t.Fatalf("missing Done event")
	}
	if !truncatedMarkerSeen {
		t.Errorf("expected (truncated) marker when noTrunc=false, but not found")
	}
	if stdoutLen >= lineLen {
		t.Errorf("captured stdout length = %d, expected < %d (line should be truncated)", stdoutLen, lineLen)
	}
}

func TestStreamPipes_StartError(t *testing.T) {
	// Create a command that will fail to start (nonexistent binary)
	cmd := &exec.Cmd{
		Path: "/nonexistent/binary",
		Args: []string{"/nonexistent/binary"},
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("StderrPipe: %v", err)
	}

	ch := make(chan StreamEvent, 64)
	streamPipes(cmd, ch, stdoutPipe, stderrPipe, "host1", time.Now(), context.Background(), false)
	close(ch)

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Should get exactly one event: the start error
	require.Len(t, events, 1)
	ev := events[0]
	assert.Equal(t, "host1", ev.Host)
	assert.True(t, ev.Done)
	assert.Equal(t, -1, ev.ExitCode)
	assert.Contains(t, ev.Error, "nonexistent/binary")
}

func TestStreamPipes_DeadlineExceeded(t *testing.T) {
	// Use an already-expired context paired with a command that exits
	// non-zero (without being killed by the context). This simulates the
	// case where the host command completes but the deadline has passed:
	// ProcessState.Exited() is true and ctx.Err() is DeadlineExceeded.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	cmd := exec.Command("sh", "-c", "exit 1")

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)

	ch := make(chan StreamEvent, 64)
	go func() {
		streamPipes(cmd, ch, stdout, stderr, "host1", time.Now(), ctx, false)
		close(ch)
	}()

	var doneEvent *StreamEvent
	timeout := time.After(10 * time.Second)
collect:
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				break collect
			}
			if ev.Done {
				doneEvent = &ev
				break collect
			}
		case <-timeout:
			t.Fatal("streamPipes did not complete within timeout")
		}
	}

	require.NotNil(t, doneEvent, "expected a Done event")
	assert.True(t, doneEvent.Done)
	assert.False(t, doneEvent.Success)
	assert.Equal(t, "timeout", doneEvent.Error)
}

func drainChannel(ch <-chan StreamEvent) []StreamEvent {
	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

// slowReader returns at most maxBytes per Read call from an underlying buffer.
type slowReader struct {
	data []byte
	pos  int
	maxBytes int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	remaining := r.data[r.pos:]
	if len(remaining) > r.maxBytes {
		remaining = remaining[:r.maxBytes]
	}
	if len(remaining) > len(p) {
		remaining = remaining[:len(p)]
	}
	n := copy(p, remaining)
	r.pos += n
	return n, nil
}

func TestStreamReader_SmallReadChunks(t *testing.T) {
	// Payload: an empty line, a short line, a long line (longer than N),
	// and a final line with no trailing newline.
	payload := "\nshort\n" + strings.Repeat("A", 4096) + "\nnolast"
	expected := []string{"short", strings.Repeat("A", 4096), "nolast"}

	chunkSizes := []int{1, 2, 3, 7, 64, 4096}
	for _, n := range chunkSizes {
		t.Run(fmt.Sprintf("chunk_%d", n), func(t *testing.T) {
			ch := make(chan StreamEvent, 64)
			r := &slowReader{data: []byte(payload), maxBytes: n}

			go func() {
				streamReader(r, ch, "h1", true, false)
				close(ch)
			}()

			events := drainChannel(ch)
			require.Len(t, events, len(expected),
				"expected %d line events with chunk size %d", len(expected), n)
			for i, want := range expected {
				assert.Equal(t, want, events[i].Line,
					"line %d mismatch with chunk size %d", i, n)
				assert.True(t, events[i].Stderr,
					"line %d should be stderr with chunk size %d", i, n)
			}
		})
	}
}

func TestStreamReader_EmptyInput(t *testing.T) {
	ch := make(chan StreamEvent, 8)

	go func() {
		streamReader(strings.NewReader(""), ch, "h1", false, false)
		close(ch)
	}()

	events := drainChannel(ch)
	// EOF with no data and no pending line: no events
	assert.Empty(t, events)
}

func TestStreamReader_EmptyInput_Stderr(t *testing.T) {
	ch := make(chan StreamEvent, 8)

	go func() {
		streamReader(strings.NewReader(""), ch, "h1", true, false)
		close(ch)
	}()

	events := drainChannel(ch)
	assert.Empty(t, events)
}

func TestStreamReader_ReadErrorImmediate(t *testing.T) {
	// Reader returns error on first read, no pending line.
	// Uses empty errorReader: data=empty, so first Read returns 0 + nil,
	// second Read returns 0 + error.
	errReader := &errorReader{err: fmt.Errorf("test error")}
	ch := make(chan StreamEvent, 8)

	go func() {
		streamReader(errReader, ch, "h1", false, false)
		close(ch)
	}()

	events := drainChannel(ch)
	require.Len(t, events, 1)
	assert.Equal(t, "h1", events[0].Host)
	assert.Equal(t, "read error: test error", events[0].Error)
	assert.True(t, events[0].Stderr)
}

func TestStreamReader_ReadErrorWithPendingLine(t *testing.T) {
	// Reader returns data on first read (no newline, so line stays pending),
	// then returns error on the next read. The pending line is flushed
	// before the error event.
	errReader := &errorReader{data: []byte("partial"), err: fmt.Errorf("fail")}
	ch := make(chan StreamEvent, 8)

	go func() {
		streamReader(errReader, ch, "h1", true, false)
		close(ch)
	}()

	events := drainChannel(ch)
	require.Len(t, events, 2)
	// first event: pending line flushed
	assert.Equal(t, "partial", events[0].Line)
	assert.True(t, events[0].Stderr)
	// second event: read error
	assert.Equal(t, "read error: fail", events[1].Error)
	assert.True(t, events[1].Stderr)
}

func TestStreamReader_ErrorWithTruncatedLine(t *testing.T) {
	// Two chunks of data cross maxScanLineSize boundary, triggering
	// truncation; then an error arrives. Truncated line + error emitted.
	chunk1 := strings.Repeat("X", maxScanLineSize)
	chunk2 := strings.Repeat("X", 1024)
	errReader := &chunkedErrorReader{
		chunks: [][]byte{[]byte(chunk1), []byte(chunk2)},
		err:    fmt.Errorf("fail"),
	}
	ch := make(chan StreamEvent, 8)

	go func() {
		streamReader(errReader, ch, "h1", false, false)
		close(ch)
	}()

	events := drainChannel(ch)
	require.Len(t, events, 2)
	assert.Contains(t, events[0].Line, "(truncated)")
	assert.Equal(t, "read error: fail", events[1].Error)
}

func TestStreamReader_NewlineAfterTruncation(t *testing.T) {
	// Chunk 1 exceeds maxScanLineSize → truncation triggered mid-chunk.
	// Chunk 2 ends with newline → truncated line emitted at newline boundary.
	chunk1 := strings.Repeat("X", maxScanLineSize+2048)
	chunk2 := "hello\n"
	r := &chunkedErrorReader{
		chunks: [][]byte{[]byte(chunk1), []byte(chunk2)},
		err:    io.EOF,
	}
	ch := make(chan StreamEvent, 8)

	go func() {
		streamReader(r, ch, "h1", true, false)
		close(ch)
	}()

	events := drainChannel(ch)
	// Truncated line emitted at newline boundary; "hello" before \n is
	// skipped because truncated flag is still set.
	require.Len(t, events, 1)
	assert.Contains(t, events[0].Line, "(truncated)")
	assert.True(t, events[0].Stderr)
}

func TestStreamReader_EOFWithTruncatedLine(t *testing.T) {
	// Data exceeds maxScanLineSize with no newline, then EOF.
	// The truncated line is flushed with "(truncated)" marker.
	bigLine := strings.Repeat("X", maxScanLineSize+1024)
	ch := make(chan StreamEvent, 8)

	go func() {
		streamReader(strings.NewReader(bigLine), ch, "h1", false, false)
		close(ch)
	}()

	events := drainChannel(ch)
	require.Len(t, events, 1)
	assert.Contains(t, events[0].Line, "(truncated)")
	// Line is maxScanLineSize+1 chars (truncation triggers after append) + marker
	assert.Equal(t, maxScanLineSize+1+len(" (truncated)"), len(events[0].Line))
}

func TestStreamReader_TruncationAtNewlineBoundary(t *testing.T) {
	// Pre-accumulate data without newlines so len(line) is close to
	// maxScanLineSize, then feed a chunk with a newline. The segment
	// before the newline, combined with the accumulated line, exceeds
	// maxScanLineSize+1 and triggers the newline-found truncation path.
	//
	// chunk1: 1000000 bytes of 'A' (no newline, accumulated into line)
	// chunk2: 48578 bytes of 'B' + newline (triggers truncation)
	// Total before newline: 1000000 + 48578 = 1048578 > maxScanLineSize+1 (1048577)
	chunk1 := strings.Repeat("A", 1000000)
	chunk2 := strings.Repeat("B", 48578) + "\n"
	r := &chunkedErrorReader{
		chunks: [][]byte{[]byte(chunk1), []byte(chunk2)},
		err:    io.EOF,
	}
	ch := make(chan StreamEvent, 8)

	go func() {
		streamReader(r, ch, "h1", false, false)
		close(ch)
	}()

	events := drainChannel(ch)
	// Expect: one truncated event (the line emitted at newline boundary)
	require.NotEmpty(t, events, "expected at least one event")
	assert.Contains(t, events[0].Line, "(truncated)", "line should be marked truncated")
}
