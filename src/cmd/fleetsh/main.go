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
	"os"
	"strings"

	"github.com/dhoard/fleetsh/internal/cli"
)

func preprocessArgs() []string {
	args := os.Args[1:]
	var result []string
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "-p" || arg == "--ping" {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				result = append(result, arg, "3")
			} else {
				result = append(result, arg, args[i+1])
				i++
			}
			i++
			continue
		}
		result = append(result, arg)
		i++
	}
	return result
}

func main() {
	os.Args = append([]string{os.Args[0]}, preprocessArgs()...)
	cli.Execute()
}
