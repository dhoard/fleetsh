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
	"context"
	"net"
	"os/user"
	"strings"
	"testing"
	"time"

	"github.com/dhoard/fleetsh/internal/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRunner(t *testing.T) {
	r := NewRunner(5, true, false, true)

	assert.Equal(t, 5, r.Parallel)
	assert.True(t, r.FailFast)
	assert.False(t, r.DryRun)
	assert.True(t, r.NoTrunc)
}

func TestBuildTasks_CommandMode(t *testing.T) {
	hosts := []*inventory.ResolvedHost{
		{
			Host:  &inventory.Host{Name: "10.0.0.1", Alias: "web", User: "admin", Port: 22},
			Group: "production",
		},
	}

	timeout := 30 * time.Second
	tasks := BuildTasks(hosts, "uptime", nil, timeout, false)

	require.Len(t, tasks, 1)
	task := tasks[0]

	assert.Same(t, hosts[0], task.Host)
	assert.Equal(t, "web", task.HostConfig.Alias)
	assert.Equal(t, "10.0.0.1", task.HostConfig.Hostname)
	assert.Equal(t, 22, task.HostConfig.Port)
	assert.Equal(t, "admin", task.HostConfig.Username)
	assert.Equal(t, "uptime", task.Command)
	assert.Nil(t, task.Script)
	assert.Equal(t, timeout, task.Timeout)
	assert.False(t, task.IsScript)
}

func TestBuildTasks_ScriptMode(t *testing.T) {
	hosts := []*inventory.ResolvedHost{
		{
			Host:  &inventory.Host{Name: "node1", User: "deploy"},
			Group: "workers",
		},
	}

	script := []byte("#!/bin/sh\necho hello")
	timeout := 60 * time.Second
	tasks := BuildTasks(hosts, "", script, timeout, true)

	require.Len(t, tasks, 1)
	task := tasks[0]

	assert.Equal(t, "", task.Command)
	assert.Equal(t, script, task.Script)
	assert.True(t, task.IsScript)
}

func TestBuildTasks_EmptyHosts(t *testing.T) {
	tasks := BuildTasks(nil, "cmd", nil, time.Second, false)

	assert.NotNil(t, tasks)
	assert.Empty(t, tasks)
}

func TestBuildTasks_MultipleHosts(t *testing.T) {
	hosts := []*inventory.ResolvedHost{
		{Host: &inventory.Host{Name: "h1", Alias: "a1"}, Group: "g1"},
		{Host: &inventory.Host{Name: "h2", Alias: "a2"}, Group: "g2"},
		{Host: &inventory.Host{Name: "h3", Alias: "a3"}, Group: "g3"},
	}

	tasks := BuildTasks(hosts, "date", nil, 10*time.Second, false)

	assert.Len(t, tasks, 3)
	for i, task := range tasks {
		assert.Same(t, hosts[i], task.Host)
		assert.Equal(t, hosts[i].Host.Alias, task.HostConfig.Alias)
	}
}

func TestBuildTasks_DefaultUser_WhenHostHasNoUser(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)

	hosts := []*inventory.ResolvedHost{
		{Host: &inventory.Host{Name: "h1"}},            // no User, no Alias
		{Host: &inventory.Host{Name: "h2", User: ""}},  // explicit empty User
	}

	tasks := BuildTasks(hosts, "echo ok", nil, 5*time.Second, false)

	assert.Len(t, tasks, 2)
	assert.Equal(t, currentUser.Username, tasks[0].HostConfig.Username)
	assert.Equal(t, currentUser.Username, tasks[1].HostConfig.Username)
}

func TestBuildTasks_HostUserOverridesDefault(t *testing.T) {
	hosts := []*inventory.ResolvedHost{
		{Host: &inventory.Host{Name: "h1", User: "customuser"}},
	}

	tasks := BuildTasks(hosts, "id", nil, time.Second, false)

	assert.Len(t, tasks, 1)
	assert.Equal(t, "customuser", tasks[0].HostConfig.Username)
}

func TestBuildTasks_SSHArgs(t *testing.T) {
	hosts := []*inventory.ResolvedHost{
		{Host: &inventory.Host{Name: "h1", SSHArgs: []string{"-i", "/key.pem", "-o", "StrictHostKeyChecking=no"}}},
	}

	tasks := BuildTasks(hosts, "ls", nil, time.Second, false)

	assert.Len(t, tasks, 1)
	assert.Equal(t, []string{"-i", "/key.pem", "-o", "StrictHostKeyChecking=no"}, tasks[0].HostConfig.SSHArgs)
}

func TestBuildTasks_NilSSHArgs(t *testing.T) {
	hosts := []*inventory.ResolvedHost{
		{Host: &inventory.Host{Name: "h1"}},
	}

	tasks := BuildTasks(hosts, "ls", nil, time.Second, false)

	assert.Len(t, tasks, 1)
	assert.Nil(t, tasks[0].HostConfig.SSHArgs)
}

func TestBuildTasks_ZeroPort(t *testing.T) {
	hosts := []*inventory.ResolvedHost{
		{Host: &inventory.Host{Name: "h1", Port: 0}},
	}

	tasks := BuildTasks(hosts, "cmd", nil, time.Second, false)

	assert.Len(t, tasks, 1)
	assert.Equal(t, 0, tasks[0].HostConfig.Port)
}

func TestBuildTasks_NonZeroPort(t *testing.T) {
	hosts := []*inventory.ResolvedHost{
		{Host: &inventory.Host{Name: "h1", Port: 2222}},
	}

	tasks := BuildTasks(hosts, "cmd", nil, time.Second, false)

	assert.Len(t, tasks, 1)
	assert.Equal(t, 2222, tasks[0].HostConfig.Port)
}

func collectDryRunEvents(t *testing.T, r *Runner, tasks []Task) []StreamEvent {
	t.Helper()
	ch := make(chan StreamEvent, len(tasks))
	r.dryRunStream(ch, tasks)
	close(ch)
	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func makeTask(alias, group, command string, isScript bool) Task {
	return Task{
		HostConfig: HostConfig{
			Alias:    alias,
			Hostname: alias,
			Username: "testuser",
		},
		Host:     &inventory.ResolvedHost{Host: &inventory.Host{Name: alias, Alias: alias}, Group: group},
		Command:  command,
		IsScript: isScript,
	}
}

func TestDoTCPPing_OpenLocalPort(t *testing.T) {
	// Positive subtest: listener on a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)

	origPorts := tcpPingPorts
	tcpPingPorts = []int{addr.Port}
	defer func() { tcpPingPorts = origPorts }()

	t.Run("success", func(t *testing.T) {
		ok, line, exitCode := doTCPPing(context.Background(), "127.0.0.1", 2)
		assert.True(t, ok)
		assert.Contains(t, line, "ok=2")
		assert.Equal(t, 0, exitCode)
	})

	// Negative subtest: port 1 is not open.
	tcpPingPorts = []int{1}
	t.Run("failure", func(t *testing.T) {
		ok, line, exitCode := doTCPPing(context.Background(), "127.0.0.1", 1)
		assert.False(t, ok)
		assert.Contains(t, line, "tcp ping failed")
		assert.Equal(t, 1, exitCode)
	})
}

func TestDryRunStream_EmptyTasks(t *testing.T) {
	r := &Runner{}
	events := collectDryRunEvents(t, r, nil)
	assert.Empty(t, events)
}

func TestDryRunStream_SingleCommand(t *testing.T) {
	r := &Runner{}
	tasks := []Task{makeTask("web", "prod", "uptime", false)}

	events := collectDryRunEvents(t, r, tasks)

	require.Len(t, events, 1)
	ev := events[0]
	assert.Equal(t, "web", ev.Host)
	assert.Equal(t, "prod", ev.Group)
	assert.True(t, ev.Done)
	assert.True(t, ev.Success)
	assert.Contains(t, ev.Line, "Host: web")
	assert.Contains(t, ev.Line, "Command: uptime")
	assert.NotContains(t, ev.Line, "Mode: script")
}

func TestDryRunStream_SingleScript(t *testing.T) {
	r := &Runner{}
	tasks := []Task{makeTask("db", "staging", "", true)}

	events := collectDryRunEvents(t, r, tasks)

	require.Len(t, events, 1)
	ev := events[0]
	assert.Equal(t, "db", ev.Host)
	assert.Equal(t, "staging", ev.Group)
	assert.True(t, ev.Done)
	assert.True(t, ev.Success)
	assert.Contains(t, ev.Line, "Host: db")
	assert.Contains(t, ev.Line, "Mode: script")
	assert.NotContains(t, ev.Line, "Command:")
}

func TestDryRunStream_MultipleTasks(t *testing.T) {
	r := &Runner{}
	tasks := []Task{
		makeTask("web", "prod", "uptime", false),
		makeTask("db", "staging", "", true),
		makeTask("cache", "prod", "date", false),
	}

	events := collectDryRunEvents(t, r, tasks)

	require.Len(t, events, 3)

	// verify order matches tasks
	assert.Equal(t, "web", events[0].Host)
	assert.Equal(t, "prod", events[0].Group)
	assert.Contains(t, events[0].Line, "Command: uptime")

	assert.Equal(t, "db", events[1].Host)
	assert.Equal(t, "staging", events[1].Group)
	assert.Contains(t, events[1].Line, "Mode: script")

	assert.Equal(t, "cache", events[2].Host)
	assert.Equal(t, "prod", events[2].Group)
	assert.Contains(t, events[2].Line, "Command: date")
}

func TestDryRunStream_AllEventsDoneAndSuccessful(t *testing.T) {
	r := &Runner{}
	tasks := []Task{
		makeTask("h1", "", "cmd", false),
		makeTask("h2", "", "", true),
	}

	events := collectDryRunEvents(t, r, tasks)

	for _, ev := range events {
		assert.True(t, ev.Done, "event for %s should be Done", ev.Host)
		assert.True(t, ev.Success, "event for %s should be Successful", ev.Host)
	}
}

func TestDryRunStream_NoGroup(t *testing.T) {
	r := &Runner{}
	tasks := []Task{makeTask("solo", "", "id", false)}

	events := collectDryRunEvents(t, r, tasks)

	require.Len(t, events, 1)
	assert.Empty(t, events[0].Group)
}

func TestDryRunStream_FormatDryRunIntegration(t *testing.T) {
	// Verify Line content matches FormatDryRun output exactly
	r := &Runner{}
	task := makeTask("proxy", "edge", "echo hello", false)

	events := collectDryRunEvents(t, r, []Task{task})

	require.Len(t, events, 1)
	expected := FormatDryRun(task.HostConfig, "command", "echo hello")
	assert.Equal(t, expected, events[0].Line)
}

func TestDryRunStream_ScriptContentIsPlaceholder(t *testing.T) {
	r := &Runner{}
	task := makeTask("node", "", "#!/bin/sh\necho hi", true)

	events := collectDryRunEvents(t, r, []Task{task})

	require.Len(t, events, 1)
	// Dry run for scripts must not leak script content; output uses "Mode: script"
	assert.NotContains(t, events[0].Line, "#!/bin/sh")
	assert.NotContains(t, events[0].Line, "echo hi")
	assert.Contains(t, events[0].Line, "Mode: script")
}

func TestDryRunStream_MixedModeLabels(t *testing.T) {
	r := &Runner{}
	// a task with IsScript=false and long command content
	tasks := []Task{makeTask("h1", "", "a somewhat long command", false)}

	events := collectDryRunEvents(t, r, tasks)

	require.Len(t, events, 1)
	line := events[0].Line
	// command mode shows the actual content
	assert.True(t, strings.Contains(line, "Command: a somewhat long command"),
		"expected 'Command: a somewhat long command' in line, got %q", line)
	assert.NotContains(t, line, "Mode: script")
}

func TestStream_DryRun(t *testing.T) {
	r := NewRunner(1, false, true, false)
	tasks := []Task{
		makeTask("web", "prod", "uptime", false),
		makeTask("db", "staging", "", true),
	}

	ch := r.Stream(context.Background(), tasks)

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	require.Len(t, events, 2)

	assert.Equal(t, "web", events[0].Host)
	assert.Equal(t, "prod", events[0].Group)
	assert.True(t, events[0].Done)
	assert.True(t, events[0].Success)
	assert.Contains(t, events[0].Line, "Command: uptime")

	assert.Equal(t, "db", events[1].Host)
	assert.Equal(t, "staging", events[1].Group)
	assert.True(t, events[1].Done)
	assert.True(t, events[1].Success)
	assert.Contains(t, events[1].Line, "Mode: script")
}

func TestStream_DryRunEmpty(t *testing.T) {
	r := NewRunner(4, false, true, false)

	ch := r.Stream(context.Background(), nil)

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	assert.Empty(t, events)
}

func TestPingHosts_EmptyHosts(t *testing.T) {
	ch := PingHosts(context.Background(), nil, 3, 1, false)

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	assert.Empty(t, events)
}

func TestPingHosts_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hosts := []*inventory.ResolvedHost{
		{Host: &inventory.Host{Name: "10.0.0.1", Alias: "h1"}},
	}

	ch := PingHosts(ctx, hosts, 3, 1, false)

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Cancelled context should cause immediate loop break; no events emitted.
	assert.Empty(t, events)
}

func TestPingHosts_CancelledContextFailFast(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hosts := []*inventory.ResolvedHost{
		{Host: &inventory.Host{Name: "10.0.0.1", Alias: "h1"}},
		{Host: &inventory.Host{Name: "10.0.0.2", Alias: "h2"}},
	}

	ch := PingHosts(ctx, hosts, 1, 1, true)

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	assert.Empty(t, events)
}

func TestPingHosts_ContextCancelledDuringExecution(t *testing.T) {
	// Use a context that cancels after a short delay. With an unroutable
	// host the goroutine will be blocked in ping; cancelling the context
	// should allow it to bail out promptly.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	hosts := []*inventory.ResolvedHost{
		{Host: &inventory.Host{Name: "192.0.2.1", Alias: "unreachable"}},
	}

	ch := PingHosts(ctx, hosts, 1, 1, false)

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Should get exactly one event (success or failure) for the single host.
	require.Len(t, events, 1)
	assert.True(t, events[0].Done)
	assert.Equal(t, "unreachable", events[0].Host)
}
