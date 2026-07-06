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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatDryRunCommand(t *testing.T) {
	hc := HostConfig{
		Alias:    "web",
		Hostname: "10.0.0.1",
		Port:     22,
		Username: "admin",
		SSHArgs:  []string{"-i", "/key.pem"},
	}

	out := FormatDryRun(hc, "command", "uptime")

	assert.Contains(t, out, "Host: web\n")
	assert.Contains(t, out, "Address: 10.0.0.1\n")
	assert.Contains(t, out, "User: admin\n")
	assert.Contains(t, out, "Port: 22\n")
	assert.Contains(t, out, "SSH Args: -i /key.pem\n")
	assert.Contains(t, out, "Command: uptime\n")
	assert.NotContains(t, out, "Mode: script")
}

func TestFormatDryRunScript(t *testing.T) {
	hc := HostConfig{
		Alias:    "db",
		Hostname: "db.example.com",
		Username: "root",
	}

	out := FormatDryRun(hc, "script", "<script>")

	assert.Contains(t, out, "Host: db\n")
	assert.Contains(t, out, "Address: db.example.com\n")
	assert.Contains(t, out, "User: root\n")
	assert.Contains(t, out, "Mode: script\n")
	assert.NotContains(t, out, "Command:")
	assert.NotContains(t, out, "Port:")
}

func TestFormatDryRunNoAlias(t *testing.T) {
	hc := HostConfig{
		Alias:    "",
		Hostname: "bare.example.com",
		Username: "deploy",
	}

	out := FormatDryRun(hc, "command", "date")

	assert.Contains(t, out, "Host: bare.example.com\n")
	assert.NotContains(t, out, "Address:")
}

func TestFormatDryRunAliasEqualsHostname(t *testing.T) {
	hc := HostConfig{
		Alias:    "solo",
		Hostname: "solo",
		Username: "ops",
	}

	out := FormatDryRun(hc, "command", "id")

	assert.Contains(t, out, "Host: solo\n")
	assert.NotContains(t, out, "Address:")
}

func TestFormatDryRunZeroPort(t *testing.T) {
	hc := HostConfig{
		Hostname: "h1",
		Username: "u",
		Port:     0,
	}

	out := FormatDryRun(hc, "command", "echo ok")
	assert.NotContains(t, out, "Port:")
}

func TestFormatDryRunNoSSHArgs(t *testing.T) {
	hc := HostConfig{
		Hostname: "h1",
		Username: "u",
	}

	out := FormatDryRun(hc, "script", "<script>")
	assert.NotContains(t, out, "SSH Args:")
}

func TestFormatDryRunEmptySSHArgs(t *testing.T) {
	hc := HostConfig{
		Hostname: "h1",
		Username: "u",
		SSHArgs:  []string{},
	}

	out := FormatDryRun(hc, "command", "ls")
	assert.NotContains(t, out, "SSH Args:")
}

func TestFormatDryRunEmptyContent(t *testing.T) {
	hc := HostConfig{
		Hostname: "h1",
		Username: "u",
	}

	out := FormatDryRun(hc, "command", "")

	assert.Contains(t, out, "Command: \n")
}

func TestFormatDryRunModeScriptNoContent(t *testing.T) {
	hc := HostConfig{
		Hostname: "h1",
		Username: "u",
	}

	out := FormatDryRun(hc, "script", "ignored content")

	assert.Contains(t, out, "Mode: script\n")
	assert.NotContains(t, out, "Command:")
}

func TestFormatDryRunUnknownMode(t *testing.T) {
	hc := HostConfig{
		Hostname: "h1",
		Username: "u",
	}

	out := FormatDryRun(hc, "unknown", "some content")

	// unknown mode falls to the "else" branch: Mode: script
	assert.Contains(t, out, "Mode: script\n")
	assert.NotContains(t, out, "Command:")
}

func TestFormatDryRunFormatting(t *testing.T) {
	// Verify exact format with all fields populated
	hc := HostConfig{
		Alias:    "proxy",
		Hostname: "192.168.1.1",
		Port:     2222,
		Username: "ubuntu",
		SSHArgs:  []string{"-o", "StrictHostKeyChecking=accept-new"},
	}

	out := FormatDryRun(hc, "command", "echo hello")

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	assert.Equal(t, 6, len(lines), "expected 6 lines: Host, Address, User, Port, SSH Args, Command")
	assert.Equal(t, "  Host: proxy", lines[0])
	assert.Equal(t, "  Address: 192.168.1.1", lines[1])
	assert.Equal(t, "  User: ubuntu", lines[2])
	assert.Equal(t, "  Port: 2222", lines[3])
	assert.Equal(t, "  SSH Args: -o StrictHostKeyChecking=accept-new", lines[4])
	assert.Equal(t, "  Command: echo hello", lines[5])
}
