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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dhoard/fleetsh/internal/sshrun"
)

func alignPrefix(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func StreamText(w io.Writer, version string, warning string, events <-chan sshrun.StreamEvent, maxLen int, start time.Time) []*sshrun.Result {
	if w == nil {
		w = os.Stdout
	}

	if version != "" {
		fmt.Fprintf(w, "%s | %s\n", alignPrefix("info", maxLen), version)
	}
	if warning != "" {
		fmt.Fprintf(w, "%s | %s\n", alignPrefix("WARNING", maxLen), warning)
	}

	var results []*sshrun.Result
	var current *sshrun.Result

	for ev := range events {
		if current == nil || current.Host != ev.Host {
			if current != nil {
				results = append(results, current)
			}
			current = &sshrun.Result{Host: ev.Host, Group: ev.Group}
		}

		if ev.Done {
			current.Success = ev.Success
			current.ExitCode = ev.ExitCode
			current.Error = ev.Error
			current.Duration = ev.Duration
			results = append(results, current)
			current = nil

			if ev.Type == "ping" && ev.Line != "" {
				fmt.Fprintf(w, "%s | %s\n", alignPrefix(ev.Host, maxLen), ev.Line)
				fmt.Fprintf(w, "%s | exit=%d duration=%s\n", alignPrefix(ev.Host, maxLen), ev.ExitCode, formatDuration(ev.Duration))
			} else {
				fmt.Fprintf(w, "%s | exit=%d duration=%s\n", alignPrefix(ev.Host, maxLen), ev.ExitCode, formatDuration(ev.Duration))
			}
		} else if ev.Error != "" {
			fmt.Fprintf(w, "%s | [error] %s\n", alignPrefix(ev.Host, maxLen), ev.Error)
		} else if ev.Stderr {
			fmt.Fprintf(w, "%s ! %s\n", alignPrefix(ev.Host, maxLen), ev.Line)
			current.Stderr += ev.Line + "\n"
		} else {
			fmt.Fprintf(w, "%s * %s\n", alignPrefix(ev.Host, maxLen), ev.Line)
			current.Stdout += ev.Line + "\n"
		}
	}

	if current != nil {
		results = append(results, current)
	}

	elapsed := time.Since(start)
	summary := sshrun.ComputeSummary(results)
	summary.Duration = elapsed
	summaryExit := 0
	if summary.Failed > 0 {
		summaryExit = 1
	}
	fmt.Fprintf(w, "%s | ok=%d failed=%d total=%d exit=%d duration=%dms\n", alignPrefix("summary", maxLen), summary.OK, summary.Failed, summary.Total, summaryExit, summary.Duration.Milliseconds())

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

	encoder := json.NewEncoder(w)

	if version != "" {
		encoder.Encode(ndjsonInfo{Source: "fleetsh", Type: "info", Message: version})
	}
	if warning != "" {
		encoder.Encode(ndjsonWarning{Source: "fleetsh", Type: "warning", Message: warning})
	}

	var results []*sshrun.Result
	var current *sshrun.Result

	for ev := range events {
		if current == nil || current.Host != ev.Host {
			if current != nil {
				results = append(results, current)
			}
			current = &sshrun.Result{Host: ev.Host, Group: ev.Group}
		}

		if ev.Done {
			current.Success = ev.Success
			current.ExitCode = ev.ExitCode
			current.Error = ev.Error
			current.Duration = ev.Duration
			results = append(results, current)
			current = nil

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
		} else if ev.Error != "" {
			encoder.Encode(ndjsonEvent{
				Source: "host",
				Host:   ev.Host,
				Group:  ev.Group,
				Type:   "error",
				Error:  ev.Error,
			})
		} else if ev.Stderr {
			current.Stderr += ev.Line + "\n"
			encoder.Encode(ndjsonEvent{
				Source: "host",
				Host:   ev.Host,
				Group:  ev.Group,
				Type:   "stderr",
				Line:   ev.Line,
			})
		} else {
			current.Stdout += ev.Line + "\n"
			encoder.Encode(ndjsonEvent{
				Source: "host",
				Host:   ev.Host,
				Group:  ev.Group,
				Type:   "stdout",
				Line:   ev.Line,
			})
		}
	}

	if current != nil {
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
