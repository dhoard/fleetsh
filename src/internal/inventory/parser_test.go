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

func writeTempInventory(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.ini")
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}
