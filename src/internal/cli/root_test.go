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
	"strings"
	"testing"
)

func TestAlignPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected string
	}{
		{"empty string", "", 5, "     "},
		{"short string", "a", 5, "a    "},
		{"exact width", "abc", 3, "abc"},
		{"short of width", "abc", 5, "abc  "},
		{"longer than width", "abcdef", 5, "abcde"},
		{"single char width", "x", 1, "x"},
		{"zero width", "abc", 0, ""},
		{"unicode", "a\u00e9", 5, "a\u00e9  "},
		{"spaces", "ab cd", 6, "ab cd "},
		{"all spaces", "", 3, "   "},
		{"exactly one over", "abcd", 3, "abc"},
		{"exactly one under", "ab", 3, "ab "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := alignPrefix(tt.input, tt.width)
			if result != tt.expected {
				t.Errorf("alignPrefix(%q, %d) = %q, want %q", tt.input, tt.width, result, tt.expected)
			}
		})
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

func TestBuildRootCmdFlags(t *testing.T) {
	cmd := buildRootCmd()

	flags := []struct {
		name     string
		short    string
		defValue string
	}{
		{"inventory", "i", ""},
		{"group", "g", ""},
		{"command", "c", ""},
		{"script", "s", ""},
		{"user", "u", ""},
		{"ping", "p", "false"},
		{"ping-count", "", "10"},
		{"key", "k", ""},
		{"parallel", "l", "1"},
		{"timeout", "o", "30000"},
		{"dry-run", "", "false"},
		{"json", "", "false"},
		{"fail-fast", "", "false"},
		{"insecure", "", "false"},
		{"tty", "t", "false"},
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
		// --ping-count validation
		{"ping-count 0", []string{"-g", "test", "-p", "--ping-count", "0"}, true, "--ping-count must be >= 1"},
		{"ping-count negative", []string{"-g", "test", "-p", "--ping-count", "-1"}, true, "--ping-count must be >= 1"},

		// --parallel validation
		{"parallel 0", []string{"-g", "test", "-c", "echo", "--parallel", "0"}, true, "--parallel must be >= 1"},
		{"parallel negative", []string{"-g", "test", "-c", "echo", "--parallel", "-1"}, true, "--parallel must be >= 1"},

		// --timeout validation
		{"timeout 0", []string{"-g", "test", "-c", "echo", "--timeout", "0"}, true, "--timeout must be >= 1"},
		{"timeout negative", []string{"-g", "test", "-c", "echo", "--timeout", "-1"}, true, "--timeout must be >= 1"},

		// --inventory path validation
		{"inventory missing", []string{"-g", "test", "-c", "echo", "--inventory", "/nonexistent/path"}, true, "cannot access inventory file"},

		// --key path validation
		{"key missing", []string{"-g", "test", "-c", "echo", "--key", "/nonexistent/key"}, true, "cannot access SSH key file"},

		// Mutual exclusivity
		{"ping with command", []string{"-g", "test", "-p", "-c", "echo"}, true, "--ping is mutually exclusive"},
		{"ping with script", []string{"-g", "test", "-p", "-s", "script.sh"}, true, "--ping is mutually exclusive"},

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