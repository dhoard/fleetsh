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

package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dhoard/fleetsh/internal/sshrun"
)

func TestPrintJSON(t *testing.T) {
	results := []*sshrun.Result{
		{
			Host:     "web01.example.com",
			Group:    "web",
			Success:  true,
			ExitCode: 0,
			Stdout:   "uptime output",
			Duration: 100 * time.Millisecond,
		},
		{
			Host:     "db01.example.com",
			Group:    "db",
			Success:  false,
			ExitCode: 1,
			Stderr:   "error message",
			Duration: 200 * time.Millisecond,
		},
	}

	summary := sshrun.Summary{OK: 1, Failed: 1, Total: 2}

	var buf bytes.Buffer
	PrintJSON(&buf, results, summary)

	var parsed jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, buf.String())
	}

	if len(parsed.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(parsed.Results))
	}
	if parsed.Results[0].Host != "web01.example.com" {
		t.Errorf("expected host web01.example.com, got %s", parsed.Results[0].Host)
	}
	if !parsed.Results[0].Success {
		t.Errorf("expected success=true for first result")
	}
	if parsed.Results[1].Success {
		t.Errorf("expected success=false for second result")
	}
	if parsed.Summary.OK != 1 || parsed.Summary.Failed != 1 || parsed.Summary.Total != 2 {
		t.Errorf("unexpected summary: %+v", parsed.Summary)
	}
}

func TestPrintJSONOmitsEmptyError(t *testing.T) {
	results := []*sshrun.Result{
		{
			Host:     "web01.example.com",
			Success:  true,
			ExitCode: 0,
			Stdout:   "ok",
			Duration: 50 * time.Millisecond,
		},
	}

	summary := sshrun.Summary{OK: 1, Failed: 0, Total: 1}

	var buf bytes.Buffer
	PrintJSON(&buf, results, summary)

	if strings.Contains(buf.String(), `"error"`) {
		t.Errorf("expected error field to be omitted when empty, got: %s", buf.String())
	}
}

func TestPrintJSONIncludesError(t *testing.T) {
	results := []*sshrun.Result{
		{
			Host:     "bad.example.com",
			Success:  false,
			ExitCode: -1,
			Error:    "connect failed",
			Duration: 10 * time.Millisecond,
		},
	}

	summary := sshrun.Summary{OK: 0, Failed: 1, Total: 1}

	var buf bytes.Buffer
	PrintJSON(&buf, results, summary)

	var parsed jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if parsed.Results[0].Error != "connect failed" {
		t.Errorf("expected error field, got: %s", parsed.Results[0].Error)
	}
}

func TestFormatDurationJSON(t *testing.T) {
	tests := []struct {
		input    time.Duration
		contains string
	}{
		{100 * time.Microsecond, "µs"},
		{50 * time.Millisecond, "ms"},
		{5 * time.Second, "s"},
	}

	for _, tc := range tests {
		result := formatDuration(tc.input)
		if !strings.Contains(result, tc.contains) {
			t.Errorf("formatDuration(%v) = %q, expected to contain %q", tc.input, result, tc.contains)
		}
	}
}
