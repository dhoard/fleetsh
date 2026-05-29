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
	"strings"
	"testing"
	"time"

	"github.com/dhoard/fleetsh/internal/sshrun"
)

func TestPrintResultsSuccess(t *testing.T) {
	results := []*sshrun.Result{
		{
			Host:     "web01",
			Group:    "web",
			Success:  true,
			ExitCode: 0,
			Stdout:   " 10:00:00 up 1 day",
			Duration: 123 * time.Millisecond,
		},
	}

	var buf bytes.Buffer
	PrintResults(&buf, results)

	output := buf.String()
	if !strings.Contains(output, "web01 | OK") {
		t.Errorf("expected OK status in output, got: %s", output)
	}
	if !strings.Contains(output, "exit=0") {
		t.Errorf("expected exit=0 in output, got: %s", output)
	}
	if !strings.Contains(output, "web01 |  10:00:00 up 1 day") {
		t.Errorf("expected stdout with alias prefix in output, got: %s", output)
	}
}

func TestPrintResultsFailure(t *testing.T) {
	results := []*sshrun.Result{
		{
			Host:     "db01",
			Group:    "db",
			Success:  false,
			ExitCode: 1,
			Stderr:   "error: connection refused",
			Duration: 98 * time.Millisecond,
		},
	}

	var buf bytes.Buffer
	PrintResults(&buf, results)

	output := buf.String()
	if !strings.Contains(output, "db01 | FAILED") {
		t.Errorf("expected FAILED status in output, got: %s", output)
	}
	if !strings.Contains(output, "exit=1") {
		t.Errorf("expected exit=1 in output, got: %s", output)
	}
	if !strings.Contains(output, "db01 ! error: connection refused") {
		t.Errorf("expected stderr prefix in output, got: %s", output)
	}
}

func TestPrintResultsWithError(t *testing.T) {
	results := []*sshrun.Result{
		{
			Host:     "bad",
			Success:  false,
			ExitCode: -1,
			Error:    "connect failed",
			Duration: 5 * time.Millisecond,
		},
	}

	var buf bytes.Buffer
	PrintResults(&buf, results)

	output := buf.String()
	if !strings.Contains(output, "bad | connect failed") {
		t.Errorf("expected error prefix in output, got: %s", output)
	}
	if !strings.Contains(output, "connect failed") {
		t.Errorf("expected error message in output, got: %s", output)
	}
}

func TestPrintSummary(t *testing.T) {
	summary := sshrun.Summary{OK: 8, Failed: 2, Total: 10}

	var buf bytes.Buffer
	PrintSummary(&buf, summary)

	output := buf.String()
	if !strings.Contains(output, "ok=8, failed=2, total=10, duration=") {
		t.Errorf("expected summary with duration in output, got: %s", output)
	}
}

func TestFormatDuration(t *testing.T) {
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
