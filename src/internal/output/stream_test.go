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
	assert.Contains(t, output, "| [error] timeout")
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
