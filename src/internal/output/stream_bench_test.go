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
	"io"
	"testing"
	"time"

	"github.com/dhoard/fleetsh/internal/sshrun"
)

func BenchmarkStreamTextAccum(b *testing.B) {
	const lineCount = 5000
	events := make([]sshrun.StreamEvent, 0, lineCount+1)
	for i := 0; i < lineCount; i++ {
		events = append(events, sshrun.StreamEvent{Host: "bench-host", Line: "line output data"})
	}
	events = append(events, sshrun.StreamEvent{
		Host: "bench-host", Done: true, Success: true, ExitCode: 0, Duration: 100 * time.Millisecond,
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch := make(chan sshrun.StreamEvent, lineCount+1)
		go func(c chan<- sshrun.StreamEvent) {
			for _, ev := range events {
				c <- ev
			}
			close(c)
		}(ch)
		StreamText(io.Discard, "", "", ch, 16, time.Now())
	}
}

func BenchmarkStreamJSON(b *testing.B) {
	const lineCount = 5000
	events := make([]sshrun.StreamEvent, 0, lineCount+1)
	for i := 0; i < lineCount; i++ {
		events = append(events, sshrun.StreamEvent{Host: "bench-host", Line: "line output data"})
	}
	events = append(events, sshrun.StreamEvent{
		Host: "bench-host", Done: true, Success: true, ExitCode: 0, Duration: 100 * time.Millisecond,
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch := make(chan sshrun.StreamEvent, lineCount+1)
		go func(c chan<- sshrun.StreamEvent) {
			for _, ev := range events {
				c <- ev
			}
			close(c)
		}(ch)
		StreamJSON(io.Discard, "", "", ch, time.Now())
	}
}

func BenchmarkAlignPrefix(b *testing.B) {
	b.Run("ascii_short", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			alignPrefix("ab", 16)
		}
	})
	b.Run("ascii_pad", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			alignPrefix("runner-1", 16)
		}
	})
	b.Run("unicode_pad", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			alignPrefix("a\u00e9", 16)
		}
	})
	b.Run("unicode_truncate", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			alignPrefix("a\u00e9cdefgh", 3)
		}
	})
}
