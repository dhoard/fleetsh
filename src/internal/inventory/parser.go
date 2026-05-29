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

package inventory

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

var validNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func isValidName(name string) bool {
	return validNameRegex.MatchString(name)
}

func Parse(path string) (*Inventory, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open inventory file %q: %w", path, err)
	}
	defer f.Close()

	inv := &Inventory{
		Aliases: make(map[string]*Host),
		Groups:  make(map[string]*Group),
	}

	scanner := bufio.NewScanner(f)
	var currentGroup *Group
	inHostSection := true

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inHostSection = false
			groupName := line[1 : len(line)-1]

			if !isValidName(groupName) {
				return nil, fmt.Errorf("invalid group name %q: must match [a-zA-Z0-9_-]", groupName)
			}
			if groupName == "summary" {
				return nil, fmt.Errorf("reserved group %q cannot be used in inventory", groupName)
			}
			if _, ok := inv.Groups[groupName]; ok {
				return nil, fmt.Errorf("duplicate group %q in inventory", groupName)
			}

			inv.Groups[groupName] = &Group{Name: groupName}
			currentGroup = inv.Groups[groupName]
			continue
		}

		if inHostSection {
			host := parseHostDefinition(line)
			if !isValidName(host.Alias) {
				return nil, fmt.Errorf("invalid alias %q: must match [a-zA-Z0-9_-]", host.Alias)
			}
			if host.Alias == "summary" {
				return nil, fmt.Errorf("reserved alias %q cannot be used in inventory", host.Alias)
			}
			if _, exists := inv.Aliases[host.Alias]; exists {
				return nil, fmt.Errorf("duplicate alias %q in inventory", host.Alias)
			}
			inv.Aliases[host.Alias] = host
		} else {
			alias := strings.Fields(line)[0]
			if !isValidName(alias) {
				return nil, fmt.Errorf("invalid alias %q in group %q: must match [a-zA-Z0-9_-]", alias, currentGroup.Name)
			}
			host, ok := inv.Aliases[alias]
			if !ok {
				return nil, fmt.Errorf("unknown alias %q referenced in group %q", alias, currentGroup.Name)
			}
			currentGroup.Hosts = append(currentGroup.Hosts, host)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading inventory file: %w", err)
	}

	return inv, nil
}

func parseHostDefinition(line string) *Host {
	args := parseSSHArgs(line)
	if len(args) == 0 {
		return &Host{}
	}

	alias := args[0]
	host := &Host{Alias: alias}

	if len(args) >= 2 {
		addr := args[1]
		if atIdx := strings.LastIndex(addr, "@"); atIdx >= 0 {
			userPart := addr[:atIdx]
			hostPart := addr[atIdx+1:]
			if userPart != "" {
				host.User = userPart
			}
			addr = hostPart
		}
		host.Name, host.Port = splitHostPort(addr)
	} else {
		host.Name = alias
	}

	if len(args) > 2 {
		host.SSHArgs = args[2:]
	}

	return host
}

func parseSSHArgs(line string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune

	// Iterate by rune (not byte) so multibyte UTF-8 in SSH args is preserved.
	for _, c := range line {
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				current.WriteRune(c)
			}
		} else {
			if c == '"' || c == '\'' {
				inQuote = true
				quoteChar = c
			} else if unicode.IsSpace(c) {
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(c)
			}
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

func splitHostPort(addr string) (string, int) {
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		host := addr[:idx]
		portStr := addr[idx+1:]
		if p, err := parsePort(portStr); err == nil {
			return host, p
		}
	}
	return addr, 0
}

func parsePort(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid port")
		}
		n = n*10 + int(c-'0')
		if n > 65535 {
			return 0, fmt.Errorf("port out of range")
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("invalid port")
	}
	return n, nil
}
