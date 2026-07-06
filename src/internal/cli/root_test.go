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

package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/dhoard/fleetsh/internal/inventory"
)

func TestExitErrorIsRecognized(t *testing.T) {
	// A *exitError must carry its code and be unwrappable via errors.As, since
	// Execute relies on errors.As to map host-failure exits to code 1.
	err := error(&exitError{code: 1})

	var ee *exitError
	if !errors.As(err, &ee) {
		t.Fatalf("errors.As failed to match *exitError")
	}
	if ee.code != 1 {
		t.Errorf("exitError.code = %d, want 1", ee.code)
	}
	if ee.Error() != fmt.Sprintf("exit code %d", 1) {
		t.Errorf("exitError.Error() = %q", ee.Error())
	}
}

func TestVersion(t *testing.T) {
	if version == "" {
		t.Error("version should not be empty")
	}
	if version != "0.0.2" {
		t.Errorf("version = %q, want %q", version, "0.0.2")
	}
}

func TestVersionFlag(t *testing.T) {
	cmd := buildRootCmd()

	tests := []struct {
		name    string
		args    []string
		wantOut string
	}{
		{"-v flag", []string{"-v"}, "fleetsh v0.0.2\n"},
		{"--version flag", []string{"--version"}, "fleetsh v0.0.2\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd.SetArgs(tt.args)
			out := &strings.Builder{}
			cmd.SetOut(out)
			cmd.SetErr(&strings.Builder{})
			cmd.Execute()

			if got := out.String(); got != tt.wantOut {
				t.Errorf("output = %q, want %q", got, tt.wantOut)
			}
		})
	}
}

func TestBuildRootCmdFlags(t *testing.T) {
	cmd := buildRootCmd()

	flags := []struct {
		name     string
		short    string
		defValue string
	}{
		{"group", "g", ""},
		{"inventory", "i", ""},
		{"command", "c", ""},
		{"script", "s", ""},
		{"timeout", "t", "30000"},
		{"ping", "p", "0"},
		{"parallel", "l", "1"},
		{"dry-run", "", "false"},
		{"json", "", "false"},
		{"fail-fast", "", "false"},
		{"no-trunc", "", "false"},
	}

	for _, f := range flags {
		t.Run(f.name, func(t *testing.T) {
			flag := cmd.Flags().Lookup(f.name)
			if flag == nil {
				t.Errorf("flag --%s not found", f.name)
				return
			}
			if flag.DefValue != f.defValue {
				t.Errorf("flag --%s default = %q, want %q", f.name, flag.DefValue, f.defValue)
			}
			if f.short != "" {
				shortFlag := cmd.Flags().ShorthandLookup(f.short)
				if shortFlag == nil {
					t.Errorf("flag -%s not found", f.short)
				}
			}
		})
	}
}

func TestBuildRootCmdHasRunE(t *testing.T) {
	cmd := buildRootCmd()
	if cmd.RunE == nil {
		t.Error("cmd.RunE should not be nil")
	}
}

func TestArgumentValidation(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		expectErr  bool
		errContain string
	}{
		// --parallel validation
		{"parallel 0", []string{"-g", "test", "-c", "echo", "--parallel", "0"}, true, "--parallel must be >= 1"},
		{"parallel negative", []string{"-g", "test", "-c", "echo", "--parallel", "-1"}, true, "--parallel must be >= 1"},

		// --timeout validation
		{"timeout 0", []string{"-g", "test", "-c", "echo", "--timeout", "0"}, true, "--timeout must be >= 1"},
		{"timeout negative", []string{"-g", "test", "-c", "echo", "--timeout", "-1"}, true, "--timeout must be >= 1"},

		// --inventory path validation
		{"inventory missing", []string{"-g", "test", "-c", "echo", "--inventory", "/nonexistent/path"}, true, "cannot access inventory file"},

		// Mutual exclusivity
		{"ping with command", []string{"-g", "test", "-p", "3", "-c", "echo"}, true, "mutually exclusive"},
		{"ping with script", []string{"-g", "test", "-p", "3", "-s", "script.sh"}, true, "mutually exclusive"},

		// Required flag
		{"no action flag", []string{"-g", "test"}, true, "exactly one of --command, --script, or --ping is required"},

		// Alias + group conflict
		{"alias with group", []string{"host1", "-g", "group1", "-c", "echo"}, true, "cannot specify both an alias argument and --group"},

		// --ping must be >= 1
		{"ping zero", []string{"-g", "test", "--ping", "0"}, true, "--ping must be >= 1"},
		{"ping negative", []string{"-g", "test", "--ping", "-1"}, true, "--ping must be >= 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildRootCmd()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContain)
				} else if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errContain)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestResolveDefaultInventory_Local(t *testing.T) {
	// When .fleetsh exists in the current directory, it takes priority.
	dir := t.TempDir()
	localPath := dir + "/.fleetsh"
	if err := os.WriteFile(localPath, []byte("hosts:\n"), 0644); err != nil {
		t.Fatalf("write local .fleetsh: %v", err)
	}

	// Isolate HOME so existing user files don't interfere.
	t.Setenv("HOME", t.TempDir())

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(orig)

	result := resolveDefaultInventory()

	if result == ".fleetsh" {
		t.Error("expected resolved path other than bare .fleetsh when local file exists")
	}
}

func TestResolveDefaultInventory_Home(t *testing.T) {
	// When .fleetsh does NOT exist in cwd but exists in home, use home.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	homeDir := t.TempDir()
	homePath := homeDir + "/.fleetsh"
	if err := os.WriteFile(homePath, []byte("hosts:\n"), 0644); err != nil {
		t.Fatalf("write home .fleetsh: %v", err)
	}
	t.Setenv("HOME", homeDir)

	result := resolveDefaultInventory()

	if !strings.Contains(result, ".fleetsh") {
		t.Errorf("expected result to contain .fleetsh, got %q", result)
	}
	if result == ".fleetsh" {
		t.Error("expected resolved path, got bare .fleetsh")
	}
}

func TestResolveDefaultInventory_Fallback(t *testing.T) {
	// When neither cwd nor home has .fleetsh, fall back to bare ".fleetsh".
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	result := resolveDefaultInventory()

	if result != ".fleetsh" {
		t.Errorf("expected fallback %q, got %q", ".fleetsh", result)
	}
}

func TestMutuallyExclusiveConflict(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"none", []string{"-g", "test"}, ""},
		{"only command", []string{"-c", "echo"}, ""},
		{"only ping", []string{"-p", "3"}, ""},
		{"ping then command", []string{"-p", "3", "-c", "echo"}, "--ping, --command are mutually exclusive"},
		{"command then ping", []string{"-c", "echo", "-p", "3"}, "--command, --ping are mutually exclusive"},
		{"command then script", []string{"-c", "echo", "-s", "x.sh"}, "--command, --script are mutually exclusive"},
		{"script then command", []string{"-s", "x.sh", "-c", "echo"}, "--script, --command are mutually exclusive"},
		{"all three", []string{"-p", "3", "-c", "echo", "-s", "x.sh"}, "--ping, --command, --script are mutually exclusive"},
		{"long flags", []string{"--ping", "3", "--command", "echo"}, "--ping, --command are mutually exclusive"},
		{"equals form", []string{"--command=echo", "--ping=3"}, "--command, --ping are mutually exclusive"},
		{"short equals form", []string{"-c=echo", "-p=3"}, "--command, --ping are mutually exclusive"},
		{"duplicate command counts once", []string{"-c", "a", "-c", "b"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mutuallyExclusiveConflict(tt.args)
			if result != tt.expected {
				t.Errorf("mutuallyExclusiveConflict(%v) = %q, want %q", tt.args, result, tt.expected)
			}
		})
	}
}

func TestIsPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"[runner-.*]", true},
		{"[.*]", true},
		{"[web]", true},
		{"runner-1", false},
		{"-g web", false},
		{"[invalid", false},
		{"invalid]", false},
		{"", false},
		{"[]", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isPattern(tt.input)
			if result != tt.expected {
				t.Errorf("isPattern(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractPattern(t *testing.T) {
	tests := []struct {
		input       string
		expected    string
		expectErr   bool
		errContains string
	}{
		{"[runner-.*]", "runner-.*", false, ""},
		{"[.*]", ".*", false, ""},
		{"[web]", "web", false, ""},
		{"[  runner-.*  ]", "runner-.*", false, ""},
		{"runner-1", "", false, ""},
		{"[invalid", "", false, ""},
		{"invalid]", "", false, ""},
		{"[]", "", true, "empty pattern"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := extractPattern(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("extractPattern(%q) = %q, want %q", tt.input, result, tt.expected)
				}
			}
		})
	}
}

func TestResolveWithPatternGroup(t *testing.T) {
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{},
		Groups: map[string]*inventory.Group{
			"web-prod":    {Name: "web-prod", Hosts: []*inventory.Host{{Name: "w1"}}},
			"web-staging": {Name: "web-staging", Hosts: []*inventory.Host{{Name: "w2"}}},
		},
	}

	hosts, err := resolveWithPattern(inv, "[web-.*]", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(hosts))
	}
}

func TestResolveWithPatternGroupExactFirst(t *testing.T) {
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{},
		Groups: map[string]*inventory.Group{
			"web":      {Name: "web", Hosts: []*inventory.Host{{Name: "exact"}}},
			"web-prod": {Name: "web-prod", Hosts: []*inventory.Host{{Name: "regex"}}},
		},
	}

	hosts, err := resolveWithPattern(inv, "web", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].Host.Name != "exact" {
		t.Errorf("expected host 'exact', got %q", hosts[0].Host.Name)
	}
}

func TestResolveWithPatternGroupRegexImplicit(t *testing.T) {
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{},
		Groups: map[string]*inventory.Group{
			"web-prod": {Name: "web-prod", Hosts: []*inventory.Host{{Name: "matched"}}},
		},
	}

	hosts, err := resolveWithPattern(inv, "web-.*", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].Host.Name != "matched" {
		t.Errorf("expected host 'matched', got %q", hosts[0].Host.Name)
	}
}

func TestResolveWithPatternGroupRegexInvalidImplicit(t *testing.T) {
	// --group with a non-matching target that is also an invalid regex
	// should fail with "no hosts found" (regex compile fails in fallback).
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{},
		Groups: map[string]*inventory.Group{
			"web": {Name: "web", Hosts: []*inventory.Host{{Name: "h1"}}},
		},
	}

	_, err := resolveWithPattern(inv, "[bad", true)
	if err == nil {
		t.Error("expected error for invalid implicit regex, got nil")
	}
}

func TestResolveWithPatternAlias(t *testing.T) {
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{
			"runner-1": {Name: "r1"},
			"runner-2": {Name: "r2"},
			"web-1":    {Name: "w1"},
		},
		Groups: map[string]*inventory.Group{},
	}

	hosts, err := resolveWithPattern(inv, "[runner-.*]", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(hosts))
	}
}

func TestResolveWithPatternInvalidRegex(t *testing.T) {
	// Explicit pattern with invalid regex should fail at compilation.
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{},
		Groups:  map[string]*inventory.Group{},
	}

	_, err := resolveWithPattern(inv, "[**]", false)
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
	if !strings.Contains(err.Error(), "invalid pattern") {
		t.Errorf("error = %q, want containing 'invalid pattern'", err.Error())
	}
}

func TestResolveWithPatternNoMatchFallback(t *testing.T) {
	// Non-pattern target with no matching alias returns error.
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{},
		Groups:  map[string]*inventory.Group{},
	}

	_, err := resolveWithPattern(inv, "nonexistent", false)
	if err == nil {
		t.Error("expected error for non-matching target")
	}
	if !strings.Contains(err.Error(), "no hosts found for target") {
		t.Errorf("error = %q, want containing 'no hosts found'", err.Error())
	}
}

func TestResolveWithPatternAliasNoMatch(t *testing.T) {
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{
			"runner-1": {Name: "r1"},
		},
		Groups: map[string]*inventory.Group{},
	}

	_, err := resolveWithPattern(inv, "[web-.*]", false)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no aliases match") {
		t.Errorf("error = %q, want containing %q", err.Error(), "no aliases match")
	}
}

func TestNoArgsShowsHelp(t *testing.T) {
	// When called with no arguments and no flags, fleetsh prints help.
	cmd := buildRootCmd()
	cmd.SetArgs([]string{})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error (help), got: %v", err)
	}
	if !strings.Contains(out.String(), "Usage") {
		t.Errorf("expected help output containing 'Usage', got: %q", out.String())
	}
}

func TestResolveWithPatternGroupNoMatchNoRegex(t *testing.T) {
	// --group with a non-existent group that is not a valid regex should fail.
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{},
		Groups: map[string]*inventory.Group{
			"web": {Name: "web", Hosts: []*inventory.Host{{Name: "h1"}}},
		},
	}

	_, err := resolveWithPattern(inv, "[invalid", true)
	if err == nil {
		t.Error("expected error for invalid pattern, got nil")
	}
}

func TestResolveWithPatternEmptyBrackets(t *testing.T) {
	// "[]" is a valid pattern syntax but extractPattern rejects empty brackets.
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{},
		Groups:  map[string]*inventory.Group{},
	}

	_, err := resolveWithPattern(inv, "[]", true)
	if err == nil {
		t.Error("expected error for empty pattern brackets, got nil")
	}
	if !strings.Contains(err.Error(), "empty pattern") {
		t.Errorf("error = %q, want containing %q", err.Error(), "empty pattern")
	}
}

func TestResolveWithPatternGroupImplicitRegexNoMatch(t *testing.T) {
	// --group with a valid regex that matches no groups should fall through
	// to the "no hosts found" error.
	inv := &inventory.Inventory{
		Aliases: map[string]*inventory.Host{},
		Groups: map[string]*inventory.Group{
			"web": {Name: "web", Hosts: []*inventory.Host{{Name: "h1"}}},
		},
	}

	_, err := resolveWithPattern(inv, "nomatch-.*", true)
	if err == nil {
		t.Error("expected error for non-matching regex target, got nil")
	}
	if !strings.Contains(err.Error(), "no hosts found for target") {
		t.Errorf("error = %q, want containing %q", err.Error(), "no hosts found for target")
	}
}

func TestFlagErrorFuncMissingArgument(t *testing.T) {
	// A flag with a missing argument should produce an error via SetFlagErrorFunc.
	cmd := buildRootCmd()
	cmd.SetArgs([]string{"--command"})
	out := &strings.Builder{}
	cmd.SetOut(out)

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for missing flag argument, got nil")
	}
}

// writeTempInventory creates a temporary inventory file with the given
// content and registers cleanup. Returns the file path.
func writeTempInventory(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "fleetsh-inv-*")
	if err != nil {
		t.Fatalf("create temp inventory: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp inventory: %v", err)
	}
	f.Close()
	return f.Name()
}

// writeTempScript creates a temporary script file with the given content and
// registers cleanup. Returns the file path.
func writeTempScript(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "fleetsh-script-*")
	if err != nil {
		t.Fatalf("create temp script: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	f.Close()
	return f.Name()
}

func TestRunE_DryRunCommandWithAlias(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-c", "echo hello", "web1"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run command, got: %v", err)
	}
}

func TestRunE_DryRunCommandWithGroup(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\n\n[production]\nweb1\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-c", "uptime", "-g", "production"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run with group, got: %v", err)
	}
}

func TestRunE_DryRunScriptWithGroup(t *testing.T) {
	invContent := "node1 deploy@10.0.0.2\n\n[workers]\nnode1\n"
	invPath := writeTempInventory(t, invContent)
	scriptPath := writeTempScript(t, "#!/bin/sh\necho hello\n")

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-s", scriptPath, "-g", "workers"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run script, got: %v", err)
	}
}

func TestRunE_DryRunNoHostsFound(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-c", "echo", "nonexistent"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for no hosts found, got nil")
	} else if !strings.Contains(err.Error(), "no hosts found") {
		t.Errorf("error = %q, want containing 'no hosts found'", err.Error())
	}
}

func TestRunE_DryRunScriptFileNotFound(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-s", "/nonexistent/script.sh", "-g", "web1"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for missing script file, got nil")
	} else if !strings.Contains(err.Error(), "cannot read script file") {
		t.Errorf("error = %q, want containing 'cannot read script file'", err.Error())
	}
}

func TestRunE_DryRunJSONOutput(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "--json", "-c", "date", "web1"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run JSON output, got: %v", err)
	}
}

func TestRunE_DryRunMultipleHostsInGroup(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\nweb2 admin@10.0.0.2\nweb3 admin@10.0.0.3\n\n[production]\nweb1\nweb2\nweb3\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-c", "hostname", "-g", "production"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run with multiple hosts, got: %v", err)
	}
}

func TestRunE_DryRunWithFailFast(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "--fail-fast", "-c", "uptime", "web1"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run with fail-fast, got: %v", err)
	}
}

func TestRunE_DryRunWithNoTrunc(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "--no-trunc", "-c", "id", "web1"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run with no-trunc, got: %v", err)
	}
}

func TestRunE_DryRunPatternAlias(t *testing.T) {
	invContent := "runner-1 admin@10.0.0.1\nrunner-2 admin@10.0.0.2\nweb-1 admin@10.0.0.3\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-c", "echo ok", "[runner-.*]"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run with alias pattern, got: %v", err)
	}
}

func TestRunE_DryRunPatternGroup(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\nweb2 admin@10.0.0.2\n\n[web-prod]\nweb1\n\n[web-staging]\nweb2\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-c", "hostname", "-g", "[web-.*]"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run with group pattern, got: %v", err)
	}
}

func TestFlagErrorFunc_MutuallyExclusiveConflict(t *testing.T) {
	// When os.Args contains mutually exclusive flags and a flag error occurs,
	// the SetFlagErrorFunc should detect the conflict and return a conflict
	// error message instead of the generic flag error.
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Set os.Args so that mutuallyExclusiveConflict(os.Args[1:]) detects
	// -c and -p as conflicting.
	os.Args = []string{"fleetsh", "-c", "echo", "-p"}

	cmd := buildRootCmd()
	// Cobra sees --command without a value → triggers flag error
	cmd.SetArgs([]string{"--command"})
	out := &strings.Builder{}
	cmd.SetOut(out)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The conflict message should be present, not the generic flag error
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want containing 'mutually exclusive'", err.Error())
	}
}

func TestFlagErrorFunc_GenericFlagError(t *testing.T) {
	// When os.Args does NOT contain mutually exclusive flags but a flag
	// error occurs, the generic flag error should be returned.
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"fleetsh", "-g", "web"}

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"--command"})
	out := &strings.Builder{}
	cmd.SetOut(out)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should be the generic flag error, not a conflict
	if strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected conflict message in error: %q", err.Error())
	}
}

func TestRunE_DryRunWithParallel(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\nweb2 admin@10.0.0.2\n\n[prod]\nweb1\nweb2\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-c", "date", "-l", "2", "-g", "prod"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run with parallel, got: %v", err)
	}
}

func TestRunE_DryRunWithTimeout(t *testing.T) {
	invContent := "web1 admin@10.0.0.1\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-c", "id", "-t", "60000", "web1"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run with timeout, got: %v", err)
	}
}

func TestRunE_DryRunAliasWithHostname(t *testing.T) {
	// Alias different from hostname → hostDisplayName uses alias
	invContent := "web admin@10.0.0.1 -i /key.pem\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-c", "uptime", "web"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("expected nil error for dry-run with alias+hostname, got: %v", err)
	}
}

func TestRunE_HelpWithCommandFlagOnly(t *testing.T) {
	// No alias/group but command flag is set → should NOT show help,
	// should try to run (and fail at no hosts found since empty target)
	invContent := "web1 admin@10.0.0.1\n"
	invPath := writeTempInventory(t, invContent)

	cmd := buildRootCmd()
	cmd.SetArgs([]string{"-i", invPath, "--dry-run", "-c", "echo"})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	// With no alias and no group, target is empty → "no hosts found"
	if err == nil {
		t.Error("expected error for empty target, got nil")
	}
}
