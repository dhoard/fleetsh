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
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dhoard/fleetsh/internal/sshrun"
)

// alignPrefix pads or truncates s to exactly width display columns, counting by
// runes so multibyte UTF-8 characters are never split mid-rune.
func alignPrefix(s string, width int) string {
	if len(s) == utf8.RuneCountInString(s) {
		// ASCII fast path: byte length == rune count.
		if len(s) >= width {
			return s[:width]
		}
		return s + strings.Repeat(" ", width-len(s))
	}
	runes := []rune(s)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// hostAccum pairs a Result with strings.Builder for efficient line
type hostAccum struct {
	res    *sshrun.Result
	stdout strings.Builder
	stderr strings.Builder
}

// finalize writes the accumulated stdout/stderr into the Result fields.
func (a *hostAccum) finalize() {
	a.res.Stdout = a.stdout.String()
	a.res.Stderr = a.stderr.String()
}

func StreamText(w io.Writer, version string, warning string, events <-chan sshrun.StreamEvent, maxLen int, start time.Time) []*sshrun.Result {
	if w == nil {
		w = os.Stdout
	}

	alignCache := make(map[string]string)
	align := func(host string) string {
		if cached, ok := alignCache[host]; ok {
			return cached
		}
		padded := alignPrefix(host, maxLen)
		alignCache[host] = padded
		return padded
	}

	bw := bufio.NewWriterSize(w, 64*1024)
	defer bw.Flush()

	if version != "" {
		fmt.Fprintf(bw, "%s | %s\n", align("info"), version)
	}
	if warning != "" {
		fmt.Fprintf(bw, "%s | %s\n", align("WARNING"), warning)
	}

	var results []*sshrun.Result
	var current *sshrun.Result
	var accum *hostAccum

	for ev := range events {
		if current == nil || current.Host != ev.Host {
			if current != nil {
				accum.finalize()
				results = append(results, current)
			}
			current = &sshrun.Result{Host: ev.Host, Group: ev.Group}
			accum = &hostAccum{res: current}
		}

		if ev.Done {
			current.Success = ev.Success
			current.ExitCode = ev.ExitCode
			current.Error = ev.Error
			current.Duration = ev.Duration
			accum.finalize()
			results = append(results, current)
			current = nil
			accum = nil

			if ev.Type == "ping" && ev.Line != "" {
				fmt.Fprintf(bw, "%s | %s\n", align(ev.Host), ev.Line)
				fmt.Fprintf(bw, "%s | exit=%d duration=%s\n", align(ev.Host), ev.ExitCode, formatDuration(ev.Duration))
			} else {
				fmt.Fprintf(bw, "%s | exit=%d duration=%s\n", align(ev.Host), ev.ExitCode, formatDuration(ev.Duration))
			}
			bw.Flush()
		} else if ev.Error != "" {
			fmt.Fprintf(bw, "%s ! %s\n", align(ev.Host), ev.Error)
			bw.Flush()
		} else if ev.Stderr {
			fmt.Fprintf(bw, "%s ! %s\n", align(ev.Host), ev.Line)
			accum.stderr.WriteString(ev.Line)
			accum.stderr.WriteByte('\n')
			bw.Flush()
		} else {
			fmt.Fprintf(bw, "%s * %s\n", align(ev.Host), ev.Line)
			accum.stdout.WriteString(ev.Line)
			accum.stdout.WriteByte('\n')
			bw.Flush()
		}
	}

	if current != nil {
		accum.finalize()
		results = append(results, current)
	}

	elapsed := time.Since(start)
	summary := sshrun.ComputeSummary(results)
	summary.Duration = elapsed
	summaryExit := 0
	if summary.Failed > 0 {
		summaryExit = 1
	}
	fmt.Fprintf(bw, "%s | ok=%d failed=%d total=%d exit=%d duration=%dms\n", align("summary"), summary.OK, summary.Failed, summary.Total, summaryExit, summary.Duration.Milliseconds())

	return results
}

type ndjsonEvent struct {
	Source     string `json:"source"`
	Host       string `json:"host,omitempty"`
	Group      string `json:"group,omitempty"`
	Type       string `json:"type"`
	Line       string `json:"line,omitempty"`
	Error      string `json:"error,omitempty"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

type ndjsonSummary struct {
	Source   string `json:"source"`
	Type     string `json:"type"`
	OK       int    `json:"ok"`
	Failed   int    `json:"failed"`
	Total    int    `json:"total"`
	Duration int64  `json:"duration_ms"`
}

type ndjsonInfo struct {
	Source  string `json:"source"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

type ndjsonWarning struct {
	Source  string `json:"source"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

func StreamJSON(w io.Writer, version string, warning string, events <-chan sshrun.StreamEvent, start time.Time) []*sshrun.Result {
	if w == nil {
		w = os.Stdout
	}

	// Buffer the writer to batch write syscalls. Flush errors are ignored
	// to preserve current behaviour (StreamJSON does not surface writer errors).
	bw := bufio.NewWriterSize(w, 64*1024)
	defer bw.Flush()

	encoder := json.NewEncoder(bw)

	if version != "" {
		encoder.Encode(ndjsonInfo{Source: "fleetsh", Type: "info", Message: version})
	}
	if warning != "" {
		encoder.Encode(ndjsonWarning{Source: "fleetsh", Type: "warning", Message: warning})
	}

	var results []*sshrun.Result
	var current *sshrun.Result
	var accum *hostAccum

	for ev := range events {
		if current == nil || current.Host != ev.Host {
			if current != nil {
				accum.finalize()
				results = append(results, current)
			}
			current = &sshrun.Result{Host: ev.Host, Group: ev.Group}
			accum = &hostAccum{res: current}
		}

		if ev.Done {
			current.Success = ev.Success
			current.ExitCode = ev.ExitCode
			current.Error = ev.Error
			current.Duration = ev.Duration
			accum.finalize()
			results = append(results, current)
			current = nil
			accum = nil

			encoder.Encode(ndjsonEvent{
				Source:     "host",
				Host:       ev.Host,
				Group:      ev.Group,
				Type:       "done",
				Line:       ev.Line,
				ExitCode:   ev.ExitCode,
				Error:      ev.Error,
				DurationMs: ev.Duration.Milliseconds(),
			})
			bw.Flush()
		} else if ev.Error != "" {
			encoder.Encode(ndjsonEvent{
				Source: "host",
				Host:   ev.Host,
				Group:  ev.Group,
				Type:   "error",
				Error:  ev.Error,
			})
			bw.Flush()
		} else if ev.Stderr {
			accum.stderr.WriteString(ev.Line)
			accum.stderr.WriteByte('\n')
			encoder.Encode(ndjsonEvent{
				Source: "host",
				Host:   ev.Host,
				Group:  ev.Group,
				Type:   "stderr",
				Line:   ev.Line,
			})
			bw.Flush()
		} else {
			accum.stdout.WriteString(ev.Line)
			accum.stdout.WriteByte('\n')
			encoder.Encode(ndjsonEvent{
				Source: "host",
				Host:   ev.Host,
				Group:  ev.Group,
				Type:   "stdout",
				Line:   ev.Line,
			})
			bw.Flush()
		}
	}

	if current != nil {
		accum.finalize()
		results = append(results, current)
	}

	elapsed := time.Since(start)
	summary := sshrun.ComputeSummary(results)
	summary.Duration = elapsed
	encoder.Encode(ndjsonSummary{
		Source:   "fleetsh",
		Type:     "summary",
		OK:       summary.OK,
		Failed:   summary.Failed,
		Total:    summary.Total,
		Duration: summary.Duration.Milliseconds(),
	})

	return results
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Truncate(time.Millisecond).String()
}
