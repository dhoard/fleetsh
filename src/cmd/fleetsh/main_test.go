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

package main

import (
	"testing"
)

func TestPackageCompiles(t *testing.T) {
	// This test ensures the main package compiles without errors
	// The actual application logic is tested in the cli package
	// This is a compilation smoke test
	t.Log("main package compiles successfully")
}
