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
}

// maxScanLineSize bounds a single line of remote stdout/stderr. The default
// bufio.Scanner limit is 64KB, which silently truncates long lines (e.g. base64
// blobs or minified output); raise it so realistic output is captured intact.
const maxScanLineSize = 1024 * 1024

// sshArgs builds the argument list for the ssh client. All client options
// (port, connection options, and inventory-supplied SSH args) must precede the
// destination, because ssh treats the first non-option argument as the
// destination and everything after it as the remote command. The caller is
// responsible for appending the single remote-command argument last.
func sshArgs(hc HostConfig) []string {
	var args []string

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

	uniqueID := filepath.Base(localPath)
	remotePath := "/tmp/fleetsh-" + uniqueID + ".tmp"

	cmd := exec.CommandContext(ctx, "scp", scpArgs(hc, localPath, remotePath)...)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("scp failed: %w", err)
	}

	os.Remove(localPath)

	return remotePath, nil
}

func executeRemoteScript(ctx context.Context, hc HostConfig, content []byte, timeout time.Duration) <-chan StreamEvent {
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
		// control (a /tmp/fleetsh-*.tmp name from os.CreateTemp) and single-quote.
		args := sshArgs(hc)
		args = append(args, "sh", "-c", buildRemoteCommand(remotePath))

		cmd := exec.CommandContext(ctx, "ssh", args...)

		stdoutPipe, _ := cmd.StdoutPipe()
		stderrPipe, _ := cmd.StderrPipe()

		streamPipes(cmd, ch, stdoutPipe, stderrPipe, displayName, start, ctx)
	}()
	return ch
}

func StreamCommand(ctx context.Context, hc HostConfig, command string, timeout time.Duration) <-chan StreamEvent {
	return executeRemoteScript(ctx, hc, []byte(command), timeout)
}

func StreamScript(ctx context.Context, hc HostConfig, scriptContent []byte, timeout time.Duration) <-chan StreamEvent {
	return executeRemoteScript(ctx, hc, scriptContent, timeout)
}

func streamPipes(cmd *exec.Cmd, ch chan<- StreamEvent, stdoutPipe, stderrPipe io.Reader, displayName string, start time.Time, ctx context.Context) {
	if err := cmd.Start(); err != nil {
		ch <- StreamEvent{Host: displayName, Error: err.Error(), Done: true, ExitCode: -1}
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		streamReader(stdoutPipe, ch, displayName, false)
	}()

	go func() {
		defer wg.Done()
		streamReader(stderrPipe, ch, displayName, true)
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

func streamReader(r io.Reader, ch chan<- StreamEvent, displayName string, isStderr bool) {
	reader := bufio.NewReaderSize(r, 64*1024)
	line := make([]byte, 0, 1024)
	buf := make([]byte, maxScanLineSize)
	truncated := false

	for {
		n, err := reader.Read(buf)
		if n == 0 && err == io.EOF {
			if len(line) > 0 {
				if truncated {
					ch <- StreamEvent{Host: displayName, Line: string(line) + " (truncated)", Stderr: isStderr}
				} else {
					ch <- StreamEvent{Host: displayName, Line: string(line), Stderr: isStderr}
				}
			}
			break
		}
		if err != nil {
			if len(line) > 0 {
				if truncated {
					ch <- StreamEvent{Host: displayName, Line: string(line) + " (truncated)", Stderr: isStderr}
				} else {
					ch <- StreamEvent{Host: displayName, Line: string(line), Stderr: isStderr}
				}
			}
			ch <- StreamEvent{Host: displayName, Error: "read error: " + err.Error(), Stderr: true}
			break
		}

		data := buf[:n]
		for i := 0; i < len(data); i++ {
			if data[i] == '\n' {
				if len(line) > 0 {
					if truncated {
						ch <- StreamEvent{Host: displayName, Line: string(line) + " (truncated)", Stderr: isStderr}
					} else {
						ch <- StreamEvent{Host: displayName, Line: string(line), Stderr: isStderr}
					}
					line = line[:0]
					truncated = false
				}
			} else {
				if !truncated {
					line = append(line, data[i])
					if len(line) > maxScanLineSize {
						truncated = true
					}
				}
			}
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
