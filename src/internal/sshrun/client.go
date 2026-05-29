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
	KeyPath  string
	Insecure bool
	TTY      bool
}

func sshArgs(hc HostConfig) []string {
	var args []string

	if hc.Port != 0 {
		args = append(args, "-p", fmt.Sprintf("%d", hc.Port))
	}

	if hc.KeyPath != "" {
		args = append(args, "-i", hc.KeyPath)
	}

	if hc.Insecure {
		args = append(args, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null")
	}

	if hc.TTY {
		args = append(args, "-t")
	}

	args = append(args, "-o", "BatchMode=yes", "-o", "ConnectTimeout=10")

	target := hc.Hostname
	if hc.Username != "" {
		target = hc.Username + "@" + hc.Hostname
	}

	args = append(args, target)
	return args
}

func scpArgs(hc HostConfig, remotePath string) []string {
	var args []string

	if hc.Port != 0 {
		args = append(args, "-P", fmt.Sprintf("%d", hc.Port))
	}

	if hc.KeyPath != "" {
		args = append(args, "-i", hc.KeyPath)
	}

	if hc.Insecure {
		args = append(args, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null")
	}

	args = append(args, "-o", "BatchMode=yes", "-o", "ConnectTimeout=10")

	target := hc.Username + "@" + hc.Hostname + ":" + remotePath
	args = append(args, target)
	return args
}

func hostDisplayName(hc HostConfig) string {
	if hc.Alias != "" {
		return hc.Alias
	}
	return hc.Hostname
}

func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

func uploadScript(ctx context.Context, hc HostConfig, content []byte, timeout time.Duration) (string, error) {
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

	scpBaseArgs := scpArgs(hc, remotePath)
	remoteTarget := scpBaseArgs[len(scpBaseArgs)-1]
	localArgs := append(scpBaseArgs[:len(scpBaseArgs)-1], localPath)
	localArgs = append(localArgs, remoteTarget)

	cmd := exec.CommandContext(ctx, "scp", localArgs...)
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

		remotePath, err := uploadScript(ctx, hc, content, timeout)
		if err != nil {
			ch <- StreamEvent{Host: displayName, Error: err.Error(), Done: true, ExitCode: -1}
			return
		}

		remoteCmd := fmt.Sprintf(
			"trap 'rm -f %s' EXIT INT TERM; "+
				"chmod 700 %s && %s; "+
				"_exit=$?; "+
				"rm -f %s; "+
				"exit $_exit",
			remotePath, remotePath, remotePath, remotePath,
		)

		args := sshArgs(hc)
		args = append(args, "sh -c '"+escapeSingleQuotes(remoteCmd)+"'")

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
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			ch <- StreamEvent{Host: displayName, Line: scanner.Text()}
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			ch <- StreamEvent{Host: displayName, Line: scanner.Text(), Stderr: true}
		}
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

func RunCommand(ctx context.Context, hc HostConfig, command string, timeout time.Duration) *Result {
	return collectResult(StreamCommand(ctx, hc, command, timeout))
}

func RunScript(ctx context.Context, hc HostConfig, scriptContent []byte, timeout time.Duration) *Result {
	return collectResult(StreamScript(ctx, hc, scriptContent, timeout))
}

func collectResult(ch <-chan StreamEvent) *Result {
	result := &Result{}
	var stdoutBuf, stderrBuf strings.Builder

	for ev := range ch {
		result.Host = ev.Host
		if ev.Done {
			result.Success = ev.Success
			result.ExitCode = ev.ExitCode
			result.Error = ev.Error
			result.Duration = ev.Duration
		} else if ev.Error != "" {
			result.Error = ev.Error
		} else if ev.Stderr {
			stderrBuf.WriteString(ev.Line)
			stderrBuf.WriteByte('\n')
		} else {
			stdoutBuf.WriteString(ev.Line)
			stdoutBuf.WriteByte('\n')
		}
	}

	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()
	return result
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
	if hc.KeyPath != "" {
		sb.WriteString(fmt.Sprintf("  Key: %s\n", hc.KeyPath))
	}
	if mode == "command" {
		sb.WriteString(fmt.Sprintf("  Command: %s\n", content))
	} else {
		sb.WriteString("  Mode: script\n")
	}
	return sb.String()
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home + "/" + path[2:]
	}
	return path
}
