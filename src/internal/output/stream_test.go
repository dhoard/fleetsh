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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		// Rune-aware: "aé" is 2 runes, padded to width 5 -> 3 spaces.
		{"unicode pad", "a\u00e9", 5, "a\u00e9   "},
		// Rune-aware truncation must not split a multibyte rune.
		{"unicode truncate", "a\u00e9c", 2, "a\u00e9"},
		{"spaces", "ab cd", 6, "ab cd "},
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

func TestStreamTextStdout(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "runner-1", Line: "uptime output"}
		events <- sshrun.StreamEvent{Host: "runner-1", Done: true, Success: true, ExitCode: 0, Duration: 123 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	results := StreamText(&buf, "", "", events, 16, time.Now())

	assert.Len(t, results, 1)
	assert.True(t, results[0].Success)
	assert.Equal(t, "uptime output\n", results[0].Stdout)

	output := buf.String()
	assert.Contains(t, output, "runner-1")
	assert.Contains(t, output, "* uptime output")
	assert.Contains(t, output, "| exit=0 duration=123ms")
	assert.Contains(t, output, "summary")
	assert.Contains(t, output, "| ok=1 failed=0 total=1")
}

func TestStreamTextStderr(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "db01", Line: "connection refused", Stderr: true}
		events <- sshrun.StreamEvent{Host: "db01", Done: true, Success: false, ExitCode: 1, Duration: 50 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	results := StreamText(&buf, "", "", events, 16, time.Now())

	assert.Len(t, results, 1)
	assert.False(t, results[0].Success)
	assert.Equal(t, "connection refused\n", results[0].Stderr)

	output := buf.String()
	assert.Contains(t, output, "db01")
	assert.Contains(t, output, "! connection refused")
	assert.Contains(t, output, "| exit=1 duration=50ms")
	assert.Contains(t, output, "summary")
	assert.Contains(t, output, "| ok=0 failed=1 total=1")
}

func TestStreamTextError(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "bad", Error: "timeout"}
		events <- sshrun.StreamEvent{Host: "bad", Done: true, Success: false, ExitCode: -1, Duration: 30 * time.Second}
		close(events)
	}()

	var buf bytes.Buffer
	StreamText(&buf, "", "", events, 16, time.Now())

	output := buf.String()
	assert.Contains(t, output, "bad")
	assert.Contains(t, output, "! timeout")
	assert.Contains(t, output, "| exit=-1")
	assert.Contains(t, output, "summary")
	assert.Contains(t, output, "| ok=0 failed=1 total=1")
}

func TestStreamTextMultipleHosts(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Line: "line1"}
		events <- sshrun.StreamEvent{Host: "h1", Done: true, Success: true, ExitCode: 0, Duration: 10 * time.Millisecond}
		events <- sshrun.StreamEvent{Host: "h2", Line: "line2"}
		events <- sshrun.StreamEvent{Host: "h2", Done: true, Success: true, ExitCode: 0, Duration: 20 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	results := StreamText(&buf, "", "", events, 16, time.Now())

	assert.Len(t, results, 2)
	output := buf.String()
	assert.Contains(t, output, "h1")
	assert.Contains(t, output, "* line1")
	assert.Contains(t, output, "h2")
	assert.Contains(t, output, "* line2")
	assert.Contains(t, output, "summary")
	assert.Contains(t, output, "| ok=2 failed=0 total=2")
}

func TestStreamJSONStdout(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "runner-1", Group: "all", Line: "uptime output"}
		events <- sshrun.StreamEvent{Host: "runner-1", Group: "all", Done: true, Success: true, ExitCode: 0, Duration: 123 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	results := StreamJSON(&buf, "", "", events, time.Now())

	assert.Len(t, results, 1)
	assert.True(t, results[0].Success)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 3)

	var stdoutEvent ndjsonEvent
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &stdoutEvent))
	assert.Equal(t, "stdout", stdoutEvent.Type)
	assert.Equal(t, "runner-1", stdoutEvent.Host)
	assert.Equal(t, "uptime output", stdoutEvent.Line)

	var doneEvent ndjsonEvent
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &doneEvent))
	assert.Equal(t, "done", doneEvent.Type)
	assert.Equal(t, 0, doneEvent.ExitCode)

	var summary ndjsonSummary
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &summary))
	assert.Equal(t, "summary", summary.Type)
	assert.Equal(t, 1, summary.OK)
	assert.Equal(t, 0, summary.Failed)
}

func TestStreamJSONStderr(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "db01", Group: "db", Line: "error msg", Stderr: true}
		events <- sshrun.StreamEvent{Host: "db01", Group: "db", Done: true, Success: false, ExitCode: 1, Duration: 50 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	StreamJSON(&buf, "", "", events, time.Now())

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 3)

	var stderrEvent ndjsonEvent
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &stderrEvent))
	assert.Equal(t, "stderr", stderrEvent.Type)
	assert.Equal(t, "error msg", stderrEvent.Line)
}

func TestStreamJSONError(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "bad", Group: "", Error: "timeout"}
		events <- sshrun.StreamEvent{Host: "bad", Group: "", Done: true, Success: false, ExitCode: -1, Duration: 30 * time.Second}
		close(events)
	}()

	var buf bytes.Buffer
	StreamJSON(&buf, "", "", events, time.Now())

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	var errorEvent ndjsonEvent
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &errorEvent))
	assert.Equal(t, "error", errorEvent.Type)
	assert.Equal(t, "timeout", errorEvent.Error)
}

func TestStreamTextPingEvent(t *testing.T) {
	// Ping events use "ping" type and print an extra line of stats.
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{
			Host: "h1", Type: "ping",
			Line:     "min=1.000ms avg=2.000ms max=3.000ms ok=3 failed=0 total=3",
			Done:     true, Success: true, ExitCode: 0,
			Duration: 3 * time.Second,
		}
		close(events)
	}()

	var buf bytes.Buffer
	StreamText(&buf, "", "", events, 16, time.Now())

	output := buf.String()
	assert.Contains(t, output, "min=1.000ms")
	assert.Contains(t, output, "| exit=0 duration=3s")
}

func TestStreamTextPingNoLine(t *testing.T) {
	// Ping event with empty Line should skip the extra stats line.
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{
			Host: "h1", Type: "ping",
			Done: true, Success: false, ExitCode: 1,
			Duration: 50 * time.Millisecond,
		}
		close(events)
	}()

	var buf bytes.Buffer
	StreamText(&buf, "", "", events, 16, time.Now())

	output := buf.String()
	assert.Contains(t, output, "| exit=1 duration=50ms")
	assert.NotContains(t, output, "min=")
}

func TestStreamTextVersionAndWarning(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 2)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Done: true, Success: true, ExitCode: 0, Duration: 1 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	StreamText(&buf, "fleetsh v1.0", "WARNING: test", events, 16, time.Now())

	output := buf.String()
	assert.Contains(t, output, "fleetsh v1.0")
	assert.Contains(t, output, "WARNING: test")
}

func TestStreamTextNilWriter(t *testing.T) {
	// Passing nil writer should default to os.Stdout without panicking.
	events := make(chan sshrun.StreamEvent, 2)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Done: true, Success: true, ExitCode: 0, Duration: 1 * time.Millisecond}
		close(events)
	}()

	// Should not panic.
	results := StreamText(nil, "", "", events, 16, time.Now())
	assert.Len(t, results, 1)
}

func TestStreamJSONVersionAndWarning(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 2)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Done: true, Success: true, ExitCode: 0, Duration: 1 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	StreamJSON(&buf, "fleetsh v1.0", "WARNING: test", events, time.Now())

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	var info ndjsonInfo
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &info))
	assert.Equal(t, "info", info.Type)
	assert.Equal(t, "fleetsh v1.0", info.Message)

	var warn ndjsonWarning
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &warn))
	assert.Equal(t, "warning", warn.Type)
	assert.Equal(t, "WARNING: test", warn.Message)
}

func TestStreamJSONPingEvent(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{
			Host: "h1", Type: "ping",
			Line:     "min=1ms avg=2ms max=3ms",
			Done:     true, Success: true, ExitCode: 0,
			Duration: 3 * time.Second,
		}
		close(events)
	}()

	var buf bytes.Buffer
	StreamJSON(&buf, "", "", events, time.Now())

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var doneEvent ndjsonEvent
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &doneEvent))
	assert.Equal(t, "done", doneEvent.Type)
	assert.Equal(t, "min=1ms avg=2ms max=3ms", doneEvent.Line)
}

func TestStreamJSONNilWriter(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 2)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Done: true, Success: true, ExitCode: 0, Duration: 1 * time.Millisecond}
		close(events)
	}()

	// Should not panic.
	results := StreamJSON(nil, "", "", events, time.Now())
	assert.Len(t, results, 1)
}

func TestStreamTextIncompleteHost(t *testing.T) {
	// A host emits stdout lines but never a Done event.
	// The trailing partial result should still be appended.
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Line: "partial output"}
		events <- sshrun.StreamEvent{Host: "h2", Done: true, Success: true, ExitCode: 0, Duration: 1 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	results := StreamText(&buf, "", "", events, 16, time.Now())

	assert.Len(t, results, 2)
	// The incomplete host (h1) should be last since h2 completed first
	output := buf.String()
	assert.Contains(t, output, "partial output")
}

func TestStreamJSONIncompleteHost(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Line: "partial output"}
		events <- sshrun.StreamEvent{Host: "h2", Done: true, Success: true, ExitCode: 0, Duration: 1 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	results := StreamJSON(&buf, "", "", events, time.Now())

	assert.Len(t, results, 2)
}

func TestStreamJSON_OutputExactStability(t *testing.T) {
	// Captures the exact JSON output of StreamJSON with a fixed event sequence
	// and asserts byte-identical output after buffering changes.
	events := make(chan sshrun.StreamEvent, 20)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Group: "g1", Line: "stdout line 1"}
		events <- sshrun.StreamEvent{Host: "h1", Group: "g1", Line: "stderr line", Stderr: true}
		events <- sshrun.StreamEvent{Host: "h1", Group: "g1", Done: true, Success: true, ExitCode: 0, Duration: 50 * time.Millisecond}
		events <- sshrun.StreamEvent{Host: "h2", Group: "g2", Error: "conn refused"}
		events <- sshrun.StreamEvent{Host: "h2", Group: "g2", Done: true, Success: false, ExitCode: 1, Duration: 100 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results := StreamJSON(&buf, "fleetsh v0.1", "warn-msg", events, start)

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Expected: info, warning, stdout, stderr, done(h1), error, done(h2), summary
	require.Len(t, lines, 8, "expected 8 NDJSON lines")

	// Line 0: info
	var info ndjsonInfo
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &info))
	assert.Equal(t, "fleetsh", info.Source)
	assert.Equal(t, "info", info.Type)
	assert.Equal(t, "fleetsh v0.1", info.Message)

	// Line 1: warning
	var warn ndjsonWarning
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &warn))
	assert.Equal(t, "warning", warn.Type)
	assert.Equal(t, "warn-msg", warn.Message)

	// Line 2: stdout
	var stdoutEv ndjsonEvent
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &stdoutEv))
	assert.Equal(t, "stdout", stdoutEv.Type)
	assert.Equal(t, "h1", stdoutEv.Host)
	assert.Equal(t, "g1", stdoutEv.Group)
	assert.Equal(t, "stdout line 1", stdoutEv.Line)

	// Line 3: stderr
	var stderrEv ndjsonEvent
	require.NoError(t, json.Unmarshal([]byte(lines[3]), &stderrEv))
	assert.Equal(t, "stderr", stderrEv.Type)
	assert.Equal(t, "stderr line", stderrEv.Line)

	// Line 4: done h1
	var doneH1 ndjsonEvent
	require.NoError(t, json.Unmarshal([]byte(lines[4]), &doneH1))
	assert.Equal(t, "done", doneH1.Type)
	assert.Equal(t, "h1", doneH1.Host)
	assert.Equal(t, 0, doneH1.ExitCode)
	assert.Equal(t, int64(50), doneH1.DurationMs)

	// Line 5: error h2
	var errEv ndjsonEvent
	require.NoError(t, json.Unmarshal([]byte(lines[5]), &errEv))
	assert.Equal(t, "error", errEv.Type)
	assert.Equal(t, "conn refused", errEv.Error)

	// Line 6: done h2
	var doneH2 ndjsonEvent
	require.NoError(t, json.Unmarshal([]byte(lines[6]), &doneH2))
	assert.Equal(t, "done", doneH2.Type)
	assert.Equal(t, "h2", doneH2.Host)
	assert.Equal(t, 1, doneH2.ExitCode)
	assert.Equal(t, int64(100), doneH2.DurationMs)

	// Line 7: summary
	var summary ndjsonSummary
	require.NoError(t, json.Unmarshal([]byte(lines[7]), &summary))
	assert.Equal(t, "fleetsh", summary.Source)
	assert.Equal(t, "summary", summary.Type)
	assert.Equal(t, 1, summary.OK)
	assert.Equal(t, 1, summary.Failed)
	assert.Equal(t, 2, summary.Total)

	// Verify Result accumulation
	require.Len(t, results, 2)
	assert.Equal(t, "stdout line 1\n", results[0].Stdout)
	assert.Equal(t, "stderr line\n", results[0].Stderr)
	assert.Equal(t, "", results[1].Stdout)
	assert.Equal(t, "", results[1].Stderr)
}

func TestStreamJSONMultipleHosts(t *testing.T) {
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Group: "g1", Line: "out1"}
		events <- sshrun.StreamEvent{Host: "h1", Group: "g1", Done: true, Success: true, ExitCode: 0, Duration: 10 * time.Millisecond}
		events <- sshrun.StreamEvent{Host: "h2", Group: "g2", Line: "out2"}
		events <- sshrun.StreamEvent{Host: "h2", Group: "g2", Done: true, Success: true, ExitCode: 0, Duration: 20 * time.Millisecond}
		close(events)
	}()

	var buf bytes.Buffer
	results := StreamJSON(&buf, "", "", events, time.Now())

	assert.Len(t, results, 2)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	var summary ndjsonSummary
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &summary))
	assert.Equal(t, 2, summary.OK)
	assert.Equal(t, 0, summary.Failed)
	assert.Equal(t, 2, summary.Total)
}

func TestStreamText_TrailingPartialHost(t *testing.T) {
	// A host emits stdout lines but no Done event, and is the last host in the
	// channel. The trailing current != nil block must finalize and append it.
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Line: "line1"}
		events <- sshrun.StreamEvent{Host: "h1", Line: "line2"}
		close(events)
	}()

	var buf bytes.Buffer
	results := StreamText(&buf, "", "", events, 16, time.Now())

	require.Len(t, results, 1)
	assert.Equal(t, "h1", results[0].Host)
	assert.Equal(t, "line1\nline2\n", results[0].Stdout)
	assert.Empty(t, results[0].Stderr)
}

func TestStreamText_TrailingPartialHostStderr(t *testing.T) {
	// A host emits stderr lines only, no Done event, last in channel.
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Line: "err1", Stderr: true}
		close(events)
	}()

	var buf bytes.Buffer
	results := StreamText(&buf, "", "", events, 16, time.Now())

	require.Len(t, results, 1)
	assert.Equal(t, "err1\n", results[0].Stderr)
	assert.Empty(t, results[0].Stdout)
}

func TestStreamJSON_TrailingPartialHost(t *testing.T) {
	// A host emits stdout/stderr lines but no Done event, last in channel.
	events := make(chan sshrun.StreamEvent, 10)
	go func() {
		events <- sshrun.StreamEvent{Host: "h1", Group: "g1", Line: "out1"}
		events <- sshrun.StreamEvent{Host: "h1", Group: "g1", Line: "err1", Stderr: true}
		close(events)
	}()

	var buf bytes.Buffer
	results := StreamJSON(&buf, "", "", events, time.Now())

	require.Len(t, results, 1)
	assert.Equal(t, "h1", results[0].Host)
	assert.Equal(t, "g1", results[0].Group)
	assert.Equal(t, "out1\n", results[0].Stdout)
	assert.Equal(t, "err1\n", results[0].Stderr)
}
