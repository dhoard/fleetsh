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
	"fmt"
	"regexp"
	"sort"
)

type Host struct {
	Name    string
	Alias   string
	User    string
	Port    int
	SSHArgs []string
}

func (h *Host) DisplayName() string {
	if h.Alias != "" {
		return h.Alias
	}
	return h.Name
}

type ResolvedHost struct {
	Host  *Host
	Group string
}

type Group struct {
	Name  string
	Hosts []*Host
}

type Inventory struct {
	Aliases map[string]*Host
	Groups  map[string]*Group
}

func (inv *Inventory) Resolve(target string) ([]*ResolvedHost, error) {
	if host, ok := inv.Aliases[target]; ok {
		return []*ResolvedHost{{Host: host, Group: ""}}, nil
	}

	if group, ok := inv.Groups[target]; ok {
		seen := make(map[string]bool)
		var result []*ResolvedHost
		for _, h := range group.Hosts {
			key := h.DisplayName()
			if !seen[key] {
				seen[key] = true
				result = append(result, &ResolvedHost{Host: h, Group: target})
			}
		}
		return result, nil
	}

	return nil, fmt.Errorf("target %q not found in inventory", target)
}

func (inv *Inventory) ResolveGroupPattern(pattern *regexp.Regexp) ([]*ResolvedHost, error) {
	var names []string
	for name := range inv.Groups {
		if pattern.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var result []*ResolvedHost
	seen := make(map[string]bool)
	for _, name := range names {
		for _, h := range inv.Groups[name].Hosts {
			key := h.DisplayName()
			if !seen[key] {
				seen[key] = true
				result = append(result, &ResolvedHost{Host: h, Group: name})
			}
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no groups match pattern %q", pattern.String())
	}
	return result, nil
}

func (inv *Inventory) ResolveAliasPattern(pattern *regexp.Regexp) ([]*ResolvedHost, error) {
	var names []string
	for alias := range inv.Aliases {
		if pattern.MatchString(alias) {
			names = append(names, alias)
		}
	}
	sort.Strings(names)

	var result []*ResolvedHost
	seen := make(map[string]bool)
	for _, alias := range names {
		host := inv.Aliases[alias]
		key := host.DisplayName()
		if !seen[key] {
			seen[key] = true
			result = append(result, &ResolvedHost{Host: host, Group: ""})
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no aliases match pattern %q", pattern.String())
	}
	return result, nil
}
