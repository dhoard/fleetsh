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
	"bytes"
	"strings"
	"testing"
)

func BenchmarkStreamReader(b *testing.B) {
	b.Run("many-short-lines", func(b *testing.B) {
		var buf bytes.Buffer
		for i := 0; i < 10000; i++ {
			buf.WriteString(strings.Repeat("x", 80))
			buf.WriteByte('\n')
		}
		data := buf.Bytes()
		ch := make(chan StreamEvent, 1024)

		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r := bytes.NewReader(data)
			go func(c chan<- StreamEvent) {
				streamReader(r, c, "h1", false, false)
				close(c)
			}(ch)
			for range ch {
			}
			ch = make(chan StreamEvent, 1024)
		}
	})

	b.Run("one-long-line_truncated", func(b *testing.B) {
		data := bytes.Repeat([]byte("X"), 2*maxScanLineSize)
		ch := make(chan StreamEvent, 1024)

		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r := bytes.NewReader(data)
			go func(c chan<- StreamEvent) {
				streamReader(r, c, "h1", false, false)
				close(c)
			}(ch)
			for range ch {
			}
			ch = make(chan StreamEvent, 1024)
		}
	})

	b.Run("one-long-line_notrunc", func(b *testing.B) {
		data := bytes.Repeat([]byte("X"), 2*maxScanLineSize)
		ch := make(chan StreamEvent, 1024)

		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r := bytes.NewReader(data)
			go func(c chan<- StreamEvent) {
				streamReader(r, c, "h1", false, true)
				close(c)
			}(ch)
			for range ch {
			}
			ch = make(chan StreamEvent, 1024)
		}
	})

	b.Run("many-short-lines_notrunc", func(b *testing.B) {
		var buf bytes.Buffer
		for i := 0; i < 10000; i++ {
			buf.WriteString(strings.Repeat("x", 80))
			buf.WriteByte('\n')
		}
		data := buf.Bytes()
		ch := make(chan StreamEvent, 1024)

		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r := bytes.NewReader(data)
			go func(c chan<- StreamEvent) {
				streamReader(r, c, "h1", false, true)
				close(c)
			}(ch)
			for range ch {
			}
			ch = make(chan StreamEvent, 1024)
		}
	})
}
