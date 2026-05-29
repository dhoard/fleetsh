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

package sshrun

import "time"

type Result struct {
	Host     string
	Group    string
	Success  bool
	ExitCode int
	Stdout   string
	Stderr   string
	Error    string
	Duration time.Duration
}

type StreamEvent struct {
	Host     string
	Group    string
	Line     string
	Stderr   bool
	Error    string
	Done     bool
	Success  bool
	ExitCode int
	Duration time.Duration
	Type     string
}

type Summary struct {
	OK       int           `json:"ok"`
	Failed   int           `json:"failed"`
	Total    int           `json:"total"`
	Duration time.Duration `json:"duration"`
}

func ComputeSummary(results []*Result) Summary {
	var s Summary
	s.Total = len(results)
	for _, r := range results {
		if r.Success {
			s.OK++
		} else {
			s.Failed++
		}
	}
	return s
}
