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
	"fmt"
	"net"
	"os/user"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dhoard/fleetsh/internal/inventory"
	"github.com/go-ping/ping"
)

type Task struct {
	Host       *inventory.ResolvedHost
	HostConfig HostConfig
	Command    string
	Script     []byte
	Timeout    time.Duration
	IsScript   bool
}

type Runner struct {
	Parallel int
	FailFast bool
	DryRun   bool
	NoTrunc  bool
}

func NewRunner(parallel int, failFast bool, dryRun bool, noTrunc bool) *Runner {
	return &Runner{
		Parallel: parallel,
		FailFast: failFast,
		DryRun:   dryRun,
		NoTrunc:  noTrunc,
	}
}

func (r *Runner) Stream(ctx context.Context, tasks []Task) <-chan StreamEvent {
	out := make(chan StreamEvent, 64)

	go func() {
		defer close(out)

		if r.DryRun {
			r.dryRunStream(out, tasks)
			return
		}

		sem := make(chan struct{}, r.Parallel)
		var wg sync.WaitGroup
		var failed atomic.Bool

		for i := range tasks {
			if r.FailFast && failed.Load() {
				break
			}

			wg.Add(1)
			sem <- struct{}{}

			go func(task Task) {
				defer wg.Done()
				defer func() { <-sem }()

			var ch <-chan StreamEvent
			if task.IsScript {
				ch = StreamScript(ctx, task.HostConfig, task.Script, task.Timeout, r.NoTrunc)
			} else {
				ch = StreamCommand(ctx, task.HostConfig, task.Command, task.Timeout, r.NoTrunc)
			}

				for ev := range ch {
					ev.Group = task.Host.Group
					out <- ev
					if ev.Done && !ev.Success {
						failed.Store(true)
					}
				}
			}(tasks[i])
		}

		wg.Wait()
	}()

	return out
}

func (r *Runner) dryRunStream(ch chan<- StreamEvent, tasks []Task) {
	for _, task := range tasks {
		content := task.Command
		mode := "command"
		if task.IsScript {
			mode = "script"
			content = "<script>"
		}
		ch <- StreamEvent{
			Host:    task.HostConfig.Alias,
			Group:   task.Host.Group,
			Line:    FormatDryRun(task.HostConfig, mode, content),
			Done:    true,
			Success: true,
		}
	}
}

func BuildTasks(hosts []*inventory.ResolvedHost, command string, script []byte, timeout time.Duration, isScript bool) []Task {
	tasks := make([]Task, len(hosts))

	defaultUser := ""
	if u, err := user.Current(); err == nil {
		defaultUser = u.Username
	}

	for i, rh := range hosts {
		username := rh.Host.User
		if username == "" {
			username = defaultUser
		}

		hc := HostConfig{
			Alias:    rh.Host.DisplayName(),
			Hostname: rh.Host.Name,
			Port:     rh.Host.Port,
			Username: username,
			SSHArgs:  rh.Host.SSHArgs,
		}

		tasks[i] = Task{
			Host:       rh,
			HostConfig: hc,
			Command:    command,
			Script:     script,
			Timeout:    timeout,
			IsScript:   isScript,
		}
	}
	return tasks
}

func PingHosts(ctx context.Context, hosts []*inventory.ResolvedHost, count int, parallel int, failFast bool) <-chan StreamEvent {
	out := make(chan StreamEvent, 64)

	go func() {
		defer close(out)

		sem := make(chan struct{}, parallel)
		var wg sync.WaitGroup
		var failed atomic.Bool

		for i := range hosts {
			if failFast && failed.Load() {
				break
			}
			if ctx.Err() != nil {
				break
			}

			wg.Add(1)
			sem <- struct{}{}

			go func(rh *inventory.ResolvedHost) {
				defer wg.Done()
				defer func() { <-sem }()

				displayName := rh.Host.DisplayName()
				start := time.Now()
				ev := StreamEvent{
					Host: displayName,
					Done: true,
					Type: "ping",
				}

				// Attempt ICMP first. If ICMP is unavailable (e.g. raw sockets
				// are not permitted), fall back to TCP ping. A genuine ICMP
				// failure (packet loss, resolution error) is reported as-is.
				success, line, exitCode, unavailable := doICMPPing(ctx, rh.Host.Name, count)
				if !unavailable {
					ev.Success = success
					ev.Line = line
					ev.ExitCode = exitCode
					ev.Duration = time.Since(start)
					if !success {
						failed.Store(true)
					}
					out <- ev
					return
				}

				success, line, exitCode = doTCPPing(ctx, rh.Host.Name, count)
				ev.Success = success
				ev.Line = line
				ev.ExitCode = exitCode
				ev.Duration = time.Since(start)
				if !success {
					failed.Store(true)
				}
				out <- ev
			}(hosts[i])
		}

		wg.Wait()
	}()

	return out
}

// doICMPPing performs an ICMP ping. The final return value reports whether ICMP
// is unavailable (e.g. permission denied for raw sockets), in which case the
// caller should fall back to TCP ping rather than treating it as a failure.
func doICMPPing(ctx context.Context, host string, count int) (success bool, line string, exitCode int, unavailable bool) {
	pinger, err := ping.NewPinger(host)
	if err != nil {
		return false, err.Error(), -1, false
	}

	pinger.Count = count
	pinger.Timeout = time.Duration(count)*time.Second + 5*time.Second
	pinger.SetPrivileged(false)

	// go-ping v1.2.0 has no context-aware Run; stop the pinger when the
	// context is cancelled so callers can abort in-flight pings.
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			pinger.Stop()
		case <-stopped:
		}
	}()

	if err := pinger.Run(); err != nil {
		if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "operation not permitted") {
			return false, "", -1, true
		}
		return false, err.Error(), -1, false
	}

	stats := pinger.Statistics()

	if stats.PacketsRecv == 0 {
		return false, "100% packet loss", 1, false
	}

	minMs := float64(stats.MinRtt) / 1e6
	avgMs := float64(stats.AvgRtt) / 1e6
	maxMs := float64(stats.MaxRtt) / 1e6
	ok := stats.PacketsRecv
	failed := stats.PacketsSent - stats.PacketsRecv
	return true, fmt.Sprintf("min=%.3fms avg=%.3fms max=%.3fms ok=%d failed=%d total=%d", minMs, avgMs, maxMs, ok, failed, stats.PacketsSent), 0, false
}

// tcpPingPorts lists ports to probe in order during TCP ping. Exposed as a
var tcpPingPorts = []int{22, 80, 443, 8080}

func doTCPPing(ctx context.Context, host string, count int) (bool, string, int) {
	// Precompute dial addresses once per host (not per attempt).
	addrs := make([]string, len(tcpPingPorts))
	for i, port := range tcpPingPorts {
		addrs[i] = fmt.Sprintf("%s:%d", host, port)
	}
	var times []time.Duration

	var dialer net.Dialer
	for i := 0; i < count; i++ {
		if ctx.Err() != nil {
			break
		}
		for _, addr := range addrs {
			start := time.Now()
			dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			conn, err := dialer.DialContext(dialCtx, "tcp", addr)
			cancel()
			dur := time.Since(start)
			if err == nil {
				conn.Close()
				times = append(times, dur)
				break
			}
		}
		// Pace successive attempts, but don't sleep after the final iteration.
		if i < count-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	if len(times) == 0 {
		return false, "tcp ping failed (no open ports detected)", 1
	}

	minMs := float64(times[0].Nanoseconds()) / 1e6
	maxMs := minMs
	var sum float64
	for _, t := range times {
		ms := float64(t.Nanoseconds()) / 1e6
		sum += ms
		if ms < minMs {
			minMs = ms
		}
		if ms > maxMs {
			maxMs = ms
		}
	}
	avgMs := sum / float64(len(times))

	ok := len(times)
	failed := count - len(times)
	return true, fmt.Sprintf("min=%.3fms avg=%.3fms max=%.3fms ok=%d failed=%d total=%d", minMs, avgMs, maxMs, ok, failed, count), 0
}
