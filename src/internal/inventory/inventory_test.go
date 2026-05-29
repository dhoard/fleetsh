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
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveByAlias(t *testing.T) {
	host := &Host{Name: "runner-1.address.cx", Alias: "runner-1", User: "administrator"}
	inv := &Inventory{
		Aliases: map[string]*Host{
			"runner-1": host,
		},
		Groups: map[string]*Group{
			"runners": {Name: "runners", Hosts: []*Host{host}},
		},
	}

	hosts, err := inv.Resolve("runner-1")
	require.NoError(t, err)
	assert.Len(t, hosts, 1)
	assert.Equal(t, "runner-1", hosts[0].Host.Alias)
	assert.Empty(t, hosts[0].Group)
}

func TestResolveByGroup(t *testing.T) {
	host1 := &Host{Name: "web01.example.com", Alias: "web01", User: "ubuntu"}
	host2 := &Host{Name: "web02.example.com", Alias: "web02", User: "ubuntu"}
	inv := &Inventory{
		Aliases: map[string]*Host{
			"web01": host1,
			"web02": host2,
		},
		Groups: map[string]*Group{
			"web": {Name: "web", Hosts: []*Host{host1, host2}},
		},
	}

	hosts, err := inv.Resolve("web")
	require.NoError(t, err)
	assert.Len(t, hosts, 2)
	assert.Equal(t, "web", hosts[0].Group)
	assert.Equal(t, "web01", hosts[0].Host.Alias)
}

func TestResolveAllGroupDefined(t *testing.T) {
	host1 := &Host{Name: "runner-1.address.cx", Alias: "runner-1", User: "administrator"}
	host2 := &Host{Name: "runner-2.address.cx", Alias: "runner-2", User: "administrator"}
	inv := &Inventory{
		Aliases: map[string]*Host{
			"runner-1": host1,
			"runner-2": host2,
		},
		Groups: map[string]*Group{
			"all": {Name: "all", Hosts: []*Host{host1, host2}},
		},
	}

	hosts, err := inv.Resolve("all")
	require.NoError(t, err)
	assert.Len(t, hosts, 2)
	assert.Equal(t, "all", hosts[0].Group)
}

func TestResolveAllGroupNotDefined(t *testing.T) {
	inv := &Inventory{
		Aliases: map[string]*Host{
			"runner-1": {Name: "runner-1.address.cx", Alias: "runner-1"},
		},
		Groups: map[string]*Group{},
	}

	_, err := inv.Resolve("all")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveUnknownTarget(t *testing.T) {
	inv := &Inventory{
		Aliases: map[string]*Host{},
		Groups:  map[string]*Group{},
	}

	_, err := inv.Resolve("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveGroupDeduplicates(t *testing.T) {
	host := &Host{Name: "shared.example.com", Alias: "shared"}
	inv := &Inventory{
		Aliases: map[string]*Host{
			"shared": host,
		},
		Groups: map[string]*Group{
			"web": {Name: "web", Hosts: []*Host{host, host}},
		},
	}

	hosts, err := inv.Resolve("web")
	require.NoError(t, err)
	assert.Len(t, hosts, 1)
}

func TestResolveAliasPriorityOverGroup(t *testing.T) {
	host := &Host{Name: "web01.example.com", Alias: "web01", User: "ubuntu"}
	inv := &Inventory{
		Aliases: map[string]*Host{
			"web01": host,
		},
		Groups: map[string]*Group{
			"web01": {Name: "web01", Hosts: []*Host{host}},
		},
	}

	hosts, err := inv.Resolve("web01")
	require.NoError(t, err)
	assert.Len(t, hosts, 1)
	assert.Empty(t, hosts[0].Group)
}

func TestResolveGroupPattern(t *testing.T) {
	host1 := &Host{Name: "web01.example.com", Alias: "web01"}
	host2 := &Host{Name: "web02.example.com", Alias: "web02"}
	host3 := &Host{Name: "db01.example.com", Alias: "db01"}

	inv := &Inventory{
		Aliases: map[string]*Host{
			"web01": host1, "web02": host2, "db01": host3,
		},
		Groups: map[string]*Group{
			"web-prod":    {Name: "web-prod", Hosts: []*Host{host1}},
			"web-staging": {Name: "web-staging", Hosts: []*Host{host2}},
			"db-prod":     {Name: "db-prod", Hosts: []*Host{host3}},
		},
	}

	hosts, err := inv.ResolveGroupPattern(regexp.MustCompile("web-.*"))
	require.NoError(t, err)
	assert.Len(t, hosts, 2)
}

func TestResolveGroupPatternNoMatch(t *testing.T) {
	inv := &Inventory{
		Aliases: map[string]*Host{},
		Groups: map[string]*Group{
			"web-prod": {Name: "web-prod"},
		},
	}

	_, err := inv.ResolveGroupPattern(regexp.MustCompile("db-.*"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no groups match")
}

func TestResolveAliasPattern(t *testing.T) {
	host1 := &Host{Name: "runner-1.example.com", Alias: "runner-1"}
	host2 := &Host{Name: "runner-2.example.com", Alias: "runner-2"}
	host3 := &Host{Name: "web-1.example.com", Alias: "web-1"}

	inv := &Inventory{
		Aliases: map[string]*Host{
			"runner-1": host1, "runner-2": host2, "web-1": host3,
		},
		Groups: map[string]*Group{},
	}

	hosts, err := inv.ResolveAliasPattern(regexp.MustCompile("runner-.*"))
	require.NoError(t, err)
	assert.Len(t, hosts, 2)
}

func TestResolveAliasPatternNoMatch(t *testing.T) {
	inv := &Inventory{
		Aliases: map[string]*Host{
			"runner-1": {Name: "runner-1.example.com", Alias: "runner-1"},
		},
		Groups: map[string]*Group{},
	}

	_, err := inv.ResolveAliasPattern(regexp.MustCompile("web-.*"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no aliases match")
}
