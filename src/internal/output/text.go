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
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dhoard/fleetsh/internal/sshrun"
)

func PrintResults(w io.Writer, results []*sshrun.Result) {
	if w == nil {
		w = os.Stdout
	}

	for _, r := range results {
		status := "OK"
		if !r.Success {
			status = "FAILED"
		}

		if r.Stdout != "" {
			for _, line := range strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n") {
				fmt.Fprintf(w, "%s | %s\n", r.Host, line)
			}
		}

		if r.Stderr != "" {
			for _, line := range strings.Split(strings.TrimRight(r.Stderr, "\n"), "\n") {
				fmt.Fprintf(w, "%s | [stderr] %s\n", r.Host, line)
			}
		}

		if r.Error != "" {
			fmt.Fprintf(w, "%s | [error] %s\n", r.Host, r.Error)
		}

		fmt.Fprintf(w, "%s | %s exit=%d duration=%s\n", r.Host, status, r.ExitCode, formatDuration(r.Duration))
	}
}

func PrintSummary(w io.Writer, summary sshrun.Summary) {
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, "summary | ok=%d, failed=%d, total=%d, duration=%dms\n", summary.OK, summary.Failed, summary.Total, summary.Duration.Milliseconds())
}
