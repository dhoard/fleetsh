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

package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHostWithUser(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	assert.Contains(t, inv.Aliases, "runner-1")
	h := inv.Aliases["runner-1"]
	assert.Equal(t, "runner-1", h.Alias)
	assert.Equal(t, "administrator", h.User)
	assert.Equal(t, "runner-1.address.cx", h.Name)
	assert.Equal(t, 0, h.Port)
	assert.Empty(t, h.SSHArgs)
}

func TestParseHostWithSSHArgs(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx -i /path/to/key.pem -o ServerAliveInterval=60
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	h := inv.Aliases["runner-1"]
	assert.Equal(t, "administrator", h.User)
	assert.Equal(t, "runner-1.address.cx", h.Name)
	assert.Equal(t, []string{"-i", "/path/to/key.pem", "-o", "ServerAliveInterval=60"}, h.SSHArgs)
}

func TestParseHostWithMultibyteSSHArg(t *testing.T) {
	// SSH args may contain non-ASCII (multibyte UTF-8) values, e.g. a path or
	// option value. parseSSHArgs must preserve runes intact rather than
	// splitting them into individual bytes.
	content := "runner-1 administrator@runner-1.address.cx -o ProxyCommand=café-proxy\n"
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	h := inv.Aliases["runner-1"]
	assert.Equal(t, []string{"-o", "ProxyCommand=café-proxy"}, h.SSHArgs)
}

func TestParseHostWithSSHArgsIncludingTTY(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx -tt
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	h := inv.Aliases["runner-1"]
	assert.Equal(t, []string{"-tt"}, h.SSHArgs)
}

func TestParseHostWithQuotedSSHArgs(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx -o "ServerAliveInterval=60" -o "StrictHostKeyChecking=no"
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	h := inv.Aliases["runner-1"]
	assert.Equal(t, []string{"-o", "ServerAliveInterval=60", "-o", "StrictHostKeyChecking=no"}, h.SSHArgs)
}

func TestParseHostWithSingleQuotedSSHArgs(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx -o 'ServerAliveInterval=60'
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	h := inv.Aliases["runner-1"]
	assert.Equal(t, []string{"-o", "ServerAliveInterval=60"}, h.SSHArgs)
}

func TestParseHostWithPortAndSSHArgs(t *testing.T) {
	content := `web01 admin@web01.example.com:2222 -i /path/to/key.pem
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	h := inv.Aliases["web01"]
	assert.Equal(t, "admin", h.User)
	assert.Equal(t, "web01.example.com", h.Name)
	assert.Equal(t, 2222, h.Port)
	assert.Equal(t, []string{"-i", "/path/to/key.pem"}, h.SSHArgs)
}

func TestParseHostWithoutUser(t *testing.T) {
	content := `web01 web01.example.com
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	assert.Contains(t, inv.Aliases, "web01")
	h := inv.Aliases["web01"]
	assert.Equal(t, "web01", h.Alias)
	assert.Empty(t, h.User)
	assert.Equal(t, "web01.example.com", h.Name)
	assert.Equal(t, 0, h.Port)
}

func TestParseHostWithPort(t *testing.T) {
	content := `web01 admin@web01.example.com:2222
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	h := inv.Aliases["web01"]
	assert.Equal(t, "admin", h.User)
	assert.Equal(t, "web01.example.com", h.Name)
	assert.Equal(t, 2222, h.Port)
}

func TestParseHostWithoutUserWithPort(t *testing.T) {
	content := `web01 web01.example.com:2222
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	h := inv.Aliases["web01"]
	assert.Empty(t, h.User)
	assert.Equal(t, "web01.example.com", h.Name)
	assert.Equal(t, 2222, h.Port)
}

func TestParseHostEmptyUser(t *testing.T) {
	// Address like "@host" — atIdx >= 0 but userPart is empty.
	content := "web01 @web01.example.com\n"
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	h := inv.Aliases["web01"]
	assert.Equal(t, "web01", h.Alias)
	assert.Empty(t, h.User)
	assert.Equal(t, "web01.example.com", h.Name)
}

func TestParseHostAliasOnly(t *testing.T) {
	content := `localhost
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	h := inv.Aliases["localhost"]
	assert.Equal(t, "localhost", h.Alias)
	assert.Equal(t, "localhost", h.Name)
	assert.Empty(t, h.User)
	assert.Equal(t, 0, h.Port)
}

func TestParseGroupsWithAliasReferences(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx
runner-2 administrator@runner-2.address.cx
runner-3 administrator@runner-3.address.cx
runner-4 administrator@runner-4.address.cx

[runners]
runner-1
runner-2
runner-3
runner-4

[primary]
runner-1
runner-2

[secondary]
runner-3
runner-4
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	assert.Len(t, inv.Aliases, 4)
	assert.Contains(t, inv.Groups, "runners")
	assert.Contains(t, inv.Groups, "primary")
	assert.Contains(t, inv.Groups, "secondary")

	assert.Len(t, inv.Groups["runners"].Hosts, 4)
	assert.Len(t, inv.Groups["primary"].Hosts, 2)
	assert.Len(t, inv.Groups["secondary"].Hosts, 2)

	assert.Equal(t, "runner-1", inv.Groups["primary"].Hosts[0].Alias)
	assert.Equal(t, "runner-2", inv.Groups["primary"].Hosts[1].Alias)
}

func TestParseAllGroup(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx
runner-2 administrator@runner-2.address.cx

[all]
runner-1
runner-2
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	assert.Contains(t, inv.Groups, "all")
	assert.Len(t, inv.Groups["all"].Hosts, 2)
}

func TestParseCommentsAndBlankLines(t *testing.T) {
	content := `# hosts
runner-1 administrator@runner-1.address.cx

; another comment
runner-2 administrator@runner-2.address.cx

[runners]
# Comment inside group
runner-1
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	inv, err := Parse(path)
	require.NoError(t, err)

	assert.Len(t, inv.Aliases, 2)
	assert.Len(t, inv.Groups["runners"].Hosts, 1)
}

func TestParseDuplicateGroupError(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx

[web]
runner-1

[web]
runner-1
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	_, err := Parse(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate group")
}

func TestParseDuplicateAliasError(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx
runner-1 administrator@runner-2.address.cx
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	_, err := Parse(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate alias")
}

func TestParseUnknownAliasInGroupError(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx

[web]
runner-1
runner-unknown
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	_, err := Parse(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown alias")
}

func TestParseFileNotFound(t *testing.T) {
	_, err := Parse("/nonexistent/path/hosts.ini")
	assert.Error(t, err)
}

func TestParseReservedAliasSummary(t *testing.T) {
	content := `summary administrator@summary.address.cx
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	_, err := Parse(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reserved alias")
}

func TestParseReservedGroupSummary(t *testing.T) {
	content := `runner-1 administrator@runner-1.address.cx

[summary]
runner-1
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	_, err := Parse(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reserved group")
}

func TestParseInvalidAliasName(t *testing.T) {
	content := `"my host" user@host.example.com
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	_, err := Parse(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid alias")
	assert.Contains(t, err.Error(), "must match")
}

func TestParseInvalidGroupName(t *testing.T) {
	content := `runner-1 user@host.example.com

[my group]
runner-1
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	_, err := Parse(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid group name")
	assert.Contains(t, err.Error(), "must match")
}

func TestParseAliasWithRegexChars(t *testing.T) {
	content := `"web.*" user@host.example.com
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	_, err := Parse(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid alias")
}

func TestParseInvalidAliasInGroup(t *testing.T) {
	content := `runner-1 user@host.example.com

[web]
runner!1
`
	path := writeTempInventory(t, content)
	defer os.Remove(path)

	_, err := Parse(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid alias")
}

func TestParsePortInvalid(t *testing.T) {
	tests := []struct {
		name string
		port string
	}{
		{"non-numeric", "abc"},
		{"mixed", "22ab"},
		{"out of range", "70000"},
		{"zero", "0"},
		{"negative sign", "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePort(tt.port)
			assert.Error(t, err)
		})
	}
}

func TestDisplayNameEmptyAlias(t *testing.T) {
	h := &Host{Name: "bare.example.com"}
	assert.Equal(t, "bare.example.com", h.DisplayName())
}

func TestParseHostDefinition_EmptyArgs(t *testing.T) {
	// A line with only double-quotes produces zero parseSSHArgs tokens,
	// exercising the len(args) == 0 branch in parseHostDefinition.
	// The caller (Parse) will reject it as an invalid alias.
	content := `""`
	path := writeTempInventory(t, content)

	_, err := Parse(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid alias")
}

func TestParseHostDefinition_WhitespaceOnlyLine(t *testing.T) {
	// Whitespace-only lines in the host section are trimmed to empty and
	// skipped by Parse, so parseHostDefinition is never called.
	// Verify this doesn't error.
	content := "   \t  \n"
	path := writeTempInventory(t, content)

	inv, err := Parse(path)
	require.NoError(t, err)
	assert.Empty(t, inv.Aliases)
}

func TestParseScannerError(t *testing.T) {
	// Verify the scanner.Err() path by reading from a directory (not a
	// regular file). os.Open succeeds on directories but bufio.Scanner
	// will fail on Read.
	tmpDir := t.TempDir()

	_, err := Parse(tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error reading inventory file")
}

func writeTempInventory(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.ini")
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}
