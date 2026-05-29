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
	"strings"
	"testing"
	"time"
)

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
