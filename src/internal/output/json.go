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
	"io"
	"os"

	"github.com/dhoard/fleetsh/internal/sshrun"
)

type jsonResult struct {
	Host     string `json:"host"`
	Group    string `json:"group"`
	Success  bool   `json:"success"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration"`
}

type jsonOutput struct {
	Results []jsonResult   `json:"results"`
	Summary sshrun.Summary `json:"summary"`
}

func PrintJSON(w io.Writer, results []*sshrun.Result, summary sshrun.Summary) {
	if w == nil {
		w = os.Stdout
	}

	output := jsonOutput{
		Results: make([]jsonResult, len(results)),
		Summary: summary,
	}

	for i, r := range results {
		output.Results[i] = jsonResult{
			Host:     r.Host,
			Group:    r.Group,
			Success:  r.Success,
			ExitCode: r.ExitCode,
			Stdout:   r.Stdout,
			Stderr:   r.Stderr,
			Error:    r.Error,
			Duration: formatDuration(r.Duration),
		}
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.Encode(output)
}
