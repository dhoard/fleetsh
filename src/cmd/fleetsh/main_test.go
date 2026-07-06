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

package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackageCompiles(t *testing.T) {
	// This test ensures the main package compiles without errors
	// The actual application logic is tested in the cli package
	// This is a compilation smoke test
	t.Log("main package compiles successfully")
}

func TestPreprocessArgs_Empty(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh"}
	result := preprocessArgs()
	assert.Empty(t, result)
}

func TestPreprocessArgs_NoPingFlag(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh", "-g", "web", "-c", "echo", "-l", "4"}
	result := preprocessArgs()
	assert.Equal(t, []string{"-g", "web", "-c", "echo", "-l", "4"}, result)
}

func TestPreprocessArgs_PingShortNoValue(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh", "-g", "web", "-p"}
	result := preprocessArgs()
	assert.Equal(t, []string{"-g", "web", "-p", "3"}, result)
}

func TestPreprocessArgs_PingLongNoValue(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh", "-g", "web", "--ping"}
	result := preprocessArgs()
	assert.Equal(t, []string{"-g", "web", "--ping", "3"}, result)
}

func TestPreprocessArgs_PingShortWithFlagValue(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	// Next arg starts with '-', so it's not a ping count
	os.Args = []string{"fleetsh", "-g", "web", "-p", "-c"}
	result := preprocessArgs()
	assert.Equal(t, []string{"-g", "web", "-p", "3", "-c"}, result)
}

func TestPreprocessArgs_PingLongWithFlagValue(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh", "-g", "web", "--ping", "--json"}
	result := preprocessArgs()
	assert.Equal(t, []string{"-g", "web", "--ping", "3", "--json"}, result)
}

func TestPreprocessArgs_PingShortWithCount(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh", "-g", "web", "-p", "5"}
	result := preprocessArgs()
	assert.Equal(t, []string{"-g", "web", "-p", "5"}, result)
}

func TestPreprocessArgs_PingLongWithCount(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh", "-g", "web", "--ping", "10"}
	result := preprocessArgs()
	assert.Equal(t, []string{"-g", "web", "--ping", "10"}, result)
}

func TestPreprocessArgs_PingAtEnd(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh", "-p"}
	result := preprocessArgs()
	assert.Equal(t, []string{"-p", "3"}, result)
}

func TestPreprocessArgs_MixedFlags(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh", "--dry-run", "-p", "2", "-g", "prod"}
	result := preprocessArgs()
	assert.Equal(t, []string{"--dry-run", "-p", "2", "-g", "prod"}, result)
}

func TestPreprocessArgs_PingShortDefaultAtEnd(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh", "-g", "web", "-p"}
	result := preprocessArgs()
	// -p at end with no value → default "3"
	assert.Equal(t, []string{"-g", "web", "-p", "3"}, result)
}

func TestPreprocessArgs_PingShortNextIsFlag(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"fleetsh", "-p", "--fail-fast"}
	result := preprocessArgs()
	// Next arg starts with '-', so default "3" is inserted, --fail-fast preserved
	assert.Equal(t, []string{"-p", "3", "--fail-fast"}, result)
}
