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

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeSummaryAllOK(t *testing.T) {
	results := []*Result{
		{Host: "h1", Success: true, ExitCode: 0},
		{Host: "h2", Success: true, ExitCode: 0},
		{Host: "h3", Success: true, ExitCode: 0},
	}

	summary := ComputeSummary(results)
	assert.Equal(t, 3, summary.OK)
	assert.Equal(t, 0, summary.Failed)
	assert.Equal(t, 3, summary.Total)
}

func TestComputeSummaryMixed(t *testing.T) {
	results := []*Result{
		{Host: "h1", Success: true, ExitCode: 0},
		{Host: "h2", Success: false, ExitCode: 1},
		{Host: "h3", Success: true, ExitCode: 0},
		{Host: "h4", Success: false, ExitCode: 2},
	}

	summary := ComputeSummary(results)
	assert.Equal(t, 2, summary.OK)
	assert.Equal(t, 2, summary.Failed)
	assert.Equal(t, 4, summary.Total)
}

func TestComputeSummaryAllFailed(t *testing.T) {
	results := []*Result{
		{Host: "h1", Success: false, ExitCode: 1, Error: "connect failed"},
		{Host: "h2", Success: false, ExitCode: -1, Error: "timeout"},
	}

	summary := ComputeSummary(results)
	assert.Equal(t, 0, summary.OK)
	assert.Equal(t, 2, summary.Failed)
	assert.Equal(t, 2, summary.Total)
}

func TestComputeSummaryEmpty(t *testing.T) {
	results := []*Result{}

	summary := ComputeSummary(results)
	assert.Equal(t, 0, summary.OK)
	assert.Equal(t, 0, summary.Failed)
	assert.Equal(t, 0, summary.Total)
}
