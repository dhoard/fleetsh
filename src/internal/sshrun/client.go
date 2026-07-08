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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type HostConfig struct {
	Alias    string
	Hostname string
	Port     int
	Username string
	SSHArgs  []string
	TTY      bool
}

// maxScanLineSize bounds a single line of remote stdout/stderr. The default
// bufio.Scanner limit is 64KB, which silently truncates long lines (e.g. base64
// blobs or minified output); raise it so realistic output is captured intact.
const maxScanLineSize = 1024 * 1024

// streamReadChunkSize is the size of the scratch buffer passed to
// bufio.Reader.Read and the bufio reader itself. Matches typical OS pipe
// buffers so each Read returns in a single syscall.
const streamReadChunkSize = 64 * 1024

// readBufPool reuses 64KB byte slices for streamReader, avoiding a heap
// allocation per host. The pool is safe for concurrent use; each goroutine
// acquires its own slice and returns it when done.
var readBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, streamReadChunkSize)
		return &b
	},
}

// sshArgs builds the argument list for the ssh client. All client options
// (port, connection options, and inventory-supplied SSH args) must precede the
// destination, because ssh treats the first non-option argument as the
// destination and everything after it as the remote command. The caller is
// responsible for appending the single remote-command argument last.
func sshArgs(hc HostConfig) []string {
	var args []string

	if hc.TTY {
		args = append(args, "-tt")
	}

	if hc.Port != 0 {
		args = append(args, "-p", fmt.Sprintf("%d", hc.Port))
	}

	args = append(args, "-o", "BatchMode=yes", "-o", "ConnectTimeout=10")

	// Inventory-supplied SSH options must come before the destination.
	args = append(args, hc.SSHArgs...)

	target := hc.Hostname
	if hc.Username != "" {
		target = hc.Username + "@" + hc.Hostname
	}

	args = append(args, target)

	return args
}

// scpArgs builds the argument list for scp to copy a local file to remotePath
// on the host. Options (port, connection options, inventory SSH args) come
// first, then the local source path, then the remote destination last.
func scpArgs(hc HostConfig, localPath, remotePath string) []string {
	var args []string

	if hc.Port != 0 {
		args = append(args, "-P", fmt.Sprintf("%d", hc.Port))
	}

	args = append(args, "-o", "BatchMode=yes", "-o", "ConnectTimeout=10")

	// Inventory-supplied SSH options must come before the source/destination.
	args = append(args, hc.SSHArgs...)

	host := hc.Hostname
	if hc.Username != "" {
		host = hc.Username + "@" + hc.Hostname
	}
	target := host + ":" + remotePath

	args = append(args, localPath, target)

	return args
}

func hostDisplayName(hc HostConfig) string {
	if hc.Alias != "" {
		return hc.Alias
	}
	return hc.Hostname
}

// shellSingleQuote returns s wrapped in single quotes, safe for inclusion in a
// POSIX shell command. Any embedded single quote is rendered as the standard
// '\” sequence (close quote, escaped quote, reopen quote).
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// buildRemoteCommand returns the POSIX sh command that executes the uploaded
// script at remotePath and removes it afterward, preserving the script's exit
// status. remotePath is single-quoted everywhere it appears so paths with shell
// metacharacters are handled safely.
func buildRemoteCommand(remotePath string) string {
	q := shellSingleQuote(remotePath)
	return fmt.Sprintf(
		"trap 'rm -f %s' EXIT INT TERM; "+
			"chmod 700 %s && sh %s; "+
			"_exit=$?; "+
			"rm -f %s; "+
			"exit $_exit",
		q, q, q, q,
	)
}

func uploadScript(ctx context.Context, hc HostConfig, content []byte) (string, error) {
	localPath := ""
	localFile, err := os.CreateTemp("", "fleetsh-*.tmp")
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("failed to create local temp file in /tmp (%w) and could not access home directory (%v)", err, homeErr)
		}
		tmpDir := filepath.Join(home, ".tmp")
		if mkdirErr := os.MkdirAll(tmpDir, 0700); mkdirErr != nil {
			return "", fmt.Errorf("failed to create local temp file in /tmp (%w) and failed to create %s (%v)", err, tmpDir, mkdirErr)
		}
		localFile, err = os.CreateTemp(tmpDir, "fleetsh-*.tmp")
		if err != nil {
			return "", fmt.Errorf("failed to create local temp file in /tmp (%w) and failed to create temp file in %s (%v)", err, tmpDir, err)
		}
	}
	localPath = localFile.Name()
	defer os.Remove(localPath)

	if _, err := localFile.Write(content); err != nil {
		localFile.Close()
		return "", fmt.Errorf("failed to write local temp file: %w", err)
	}
	localFile.Close()

	base := filepath.Base(localPath)
	uniqueID := strings.TrimSuffix(strings.TrimPrefix(base, "fleetsh-"), ".tmp")
	remotePath := "/tmp/fleetsh-" + uniqueID + ".tmp"

	cmd := exec.CommandContext(ctx, "scp", scpArgs(hc, localPath, remotePath)...)
	if err := cmd.Run(); err != nil {
		scpErr := err

		homeDir, mkdirErr := createRemoteFallbackDir(ctx, hc)
		if mkdirErr != nil {
			return "", fmt.Errorf("scp to /tmp failed (%w) and could not create remote ~/.tmp (%v)", scpErr, mkdirErr)
		}

		fallbackPath := homeDir + "/.tmp/fleetsh-" + uniqueID + ".tmp"
		cmd = exec.CommandContext(ctx, "scp", scpArgs(hc, localPath, fallbackPath)...)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("scp to /tmp failed (%w) and scp to %s also failed (%v)", scpErr, fallbackPath, err)
		}

		os.Remove(localPath)
		return fallbackPath, nil
	}

	os.Remove(localPath)

	return remotePath, nil
}

func createRemoteFallbackDir(ctx context.Context, hc HostConfig) (string, error) {
	args := sshArgs(hc)
	args = append(args, "sh", "-c", "mkdir -p ~/.tmp && echo $HOME")

	cmd := exec.CommandContext(ctx, "ssh", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ssh mkdir ~/.tmp failed: %w", err)
	}

	home := strings.TrimSpace(string(out))
	if home == "" {
		return "", fmt.Errorf("ssh mkdir ~/.tmp: empty home directory")
	}

	return home, nil
}

func executeRemoteScript(ctx context.Context, hc HostConfig, content []byte, timeout time.Duration, noTrunc bool) <-chan StreamEvent {
	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		start := time.Now()
		displayName := hostDisplayName(hc)

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		remotePath, err := uploadScript(ctx, hc, content)
		if err != nil {
			ch <- StreamEvent{Host: displayName, Error: err.Error(), Done: true, ExitCode: -1}
			return
		}

		// The script has already been uploaded to remotePath. Execute it with
		// an explicit POSIX sh (the documented remote requirement) and clean up
		// the temp file afterward. The remote command is passed as a discrete
		// "sh", "-c", <command> argv triple rather than being pre-wrapped in an
		// outer `sh -c '...'` string, so we avoid hand-rolled quote escaping.
		// The only value interpolated into the command is remotePath, which we
		// control (a /tmp/fleetsh-<random>.tmp name derived from the local
		// os.CreateTemp name) and single-quote.
		args := sshArgs(hc)
		args = append(args, "sh", "-c", buildRemoteCommand(remotePath))

		cmd := exec.CommandContext(ctx, "ssh", args...)

		stdoutPipe, _ := cmd.StdoutPipe()
		stderrPipe, _ := cmd.StderrPipe()

		streamPipes(cmd, ch, stdoutPipe, stderrPipe, displayName, start, ctx, noTrunc)
	}()
	return ch
}

func StreamCommand(ctx context.Context, hc HostConfig, command string, timeout time.Duration, noTrunc bool) <-chan StreamEvent {
	return executeRemoteScript(ctx, hc, []byte(command), timeout, noTrunc)
}

func StreamScript(ctx context.Context, hc HostConfig, scriptContent []byte, timeout time.Duration, noTrunc bool) <-chan StreamEvent {
	return executeRemoteScript(ctx, hc, scriptContent, timeout, noTrunc)
}

func streamPipes(cmd *exec.Cmd, ch chan<- StreamEvent, stdoutPipe, stderrPipe io.Reader, displayName string, start time.Time, ctx context.Context, noTrunc bool) {
	if err := cmd.Start(); err != nil {
		ch <- StreamEvent{Host: displayName, Error: err.Error(), Done: true, ExitCode: -1}
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		streamReader(stdoutPipe, ch, displayName, false, noTrunc)
	}()
	
	go func() {
		defer wg.Done()
		streamReader(stderrPipe, ch, displayName, true, noTrunc)
	}()

	wg.Wait()

	err := cmd.Wait()
	duration := time.Since(start)

	event := StreamEvent{
		Host:     displayName,
		Done:     true,
		Duration: duration,
	}

	if err == nil {
		event.Success = true
		event.ExitCode = 0
	} else {
		if exitErr, ok := err.(*exec.ExitError); ok {
			event.ExitCode = exitErr.ExitCode()
		} else {
			event.ExitCode = -1
			event.Error = err.Error()
		}
		if cmd.ProcessState == nil || cmd.ProcessState.Exited() {
			if ctx.Err() == context.DeadlineExceeded {
				event.Error = "timeout"
			}
		}
	}

	ch <- event
}

// emitLine sends a single StreamEvent for a completed logical line.
func emitLine(ch chan<- StreamEvent, displayName string, isStderr bool, line []byte, truncated bool) {
	line = bytes.TrimRight(line, "\r")
	if len(line) == 0 {
		return
	}
	if truncated {
		ch <- StreamEvent{Host: displayName, Line: string(line) + " (truncated)", Stderr: isStderr}
	} else {
		ch <- StreamEvent{Host: displayName, Line: string(line), Stderr: isStderr}
	}
}

// streamReader reads from r in streamReadChunkSize chunks, splits on newlines,
// and emits one StreamEvent per logical line. A final line without a trailing
// newline is flushed at EOF. Lines longer than maxScanLineSize are capped and
// marked "(truncated)".
func streamReader(r io.Reader, ch chan<- StreamEvent, displayName string, isStderr bool, noTrunc bool) {
	bufp := readBufPool.Get().(*[]byte)
	buf := *bufp
	defer readBufPool.Put(bufp)

	reader := bufio.NewReaderSize(r, streamReadChunkSize)
	line := make([]byte, 0, 1024)
	truncated := false

	for {
		n, err := reader.Read(buf)
		if n == 0 && err == io.EOF {
			emitLine(ch, displayName, isStderr, line, truncated)
			break
		}
		if err != nil {
			emitLine(ch, displayName, isStderr, line, truncated)
			ch <- StreamEvent{Host: displayName, Error: "read error: " + err.Error(), Stderr: true}
			break
		}

		data := buf[:n]
		off := 0
		for off < len(data) {
			rel := bytes.IndexByte(data[off:], '\n')
			if rel < 0 {
				// No newline in remainder: belongs to the open line.
				seg := data[off:]
				if !truncated {
					if !noTrunc && len(line)+len(seg) > maxScanLineSize+1 {
						seg = seg[:maxScanLineSize+1-len(line)]
						line = append(line, seg...)
						truncated = true
					} else {
						line = append(line, seg...)
					}
				}
				break
			}
			// Bytes [off, off+rel) belong to the current line.
			seg := data[off : off+rel]
			if !truncated {
				if !noTrunc && len(line)+len(seg) > maxScanLineSize+1 {
					seg = seg[:maxScanLineSize+1-len(line)]
					line = append(line, seg...)
					truncated = true
				} else {
					line = append(line, seg...)
				}
			}
			if len(line) > 0 {
				emitLine(ch, displayName, isStderr, line, truncated)
				line = line[:0]
				truncated = false
			}
			off += rel + 1
		}
	}
}

func FormatDryRun(hc HostConfig, mode string, content string) string {
	displayName := hostDisplayName(hc)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  Host: %s\n", displayName))
	if hc.Alias != "" && hc.Alias != hc.Hostname {
		sb.WriteString(fmt.Sprintf("  Address: %s\n", hc.Hostname))
	}
	sb.WriteString(fmt.Sprintf("  User: %s\n", hc.Username))
	if hc.Port != 0 {
		sb.WriteString(fmt.Sprintf("  Port: %d\n", hc.Port))
	}
	if len(hc.SSHArgs) > 0 {
		sb.WriteString(fmt.Sprintf("  SSH Args: %s\n", strings.Join(hc.SSHArgs, " ")))
	}
	if mode == "command" {
		sb.WriteString(fmt.Sprintf("  Command: %s\n", content))
	} else {
		sb.WriteString("  Mode: script\n")
	}
	return sb.String()
}
