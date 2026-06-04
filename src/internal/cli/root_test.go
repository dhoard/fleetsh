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
	if version != "0.0.1" {
		t.Errorf("version = %q, want %q", version, "0.0.1")
	}
}

func TestVersionFlag(t *testing.T) {
	cmd := buildRootCmd()

	tests := []struct {
		name    string
		args    []string
		wantOut string
	}{
		{"-v flag", []string{"-v"}, "fleetsh v0.0.1\n"},
		{"--version flag", []string{"--version"}, "fleetsh v0.0.1\n"},
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
