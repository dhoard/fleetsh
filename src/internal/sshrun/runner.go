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
}

func NewRunner(parallel int, failFast bool, dryRun bool) *Runner {
	return &Runner{
		Parallel: parallel,
		FailFast: failFast,
		DryRun:   dryRun,
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
					ch = StreamScript(ctx, task.HostConfig, task.Script, task.Timeout)
				} else {
					ch = StreamCommand(ctx, task.HostConfig, task.Command, task.Timeout)
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

func (r *Runner) Run(ctx context.Context, tasks []Task) []*Result {
	events := r.Stream(ctx, tasks)
	var results []*Result

	current := &Result{}
	var stdoutBuf, stderrBuf strings.Builder

	for ev := range events {
		if current.Host == "" {
			current.Host = ev.Host
			current.Group = ev.Group
		}

		if ev.Done {
			current.Success = ev.Success
			current.ExitCode = ev.ExitCode
			current.Error = ev.Error
			current.Duration = ev.Duration
			current.Stdout = stdoutBuf.String()
			current.Stderr = stderrBuf.String()
			results = append(results, current)
			current = &Result{}
			stdoutBuf.Reset()
			stderrBuf.Reset()
		} else if ev.Error != "" {
			current.Error = ev.Error
		} else if ev.Stderr {
			stderrBuf.WriteString(ev.Line)
			stderrBuf.WriteByte('\n')
		} else {
			stdoutBuf.WriteString(ev.Line)
			stdoutBuf.WriteByte('\n')
		}
	}

	return results
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

func PingHosts(hosts []*inventory.ResolvedHost, count int, parallel int, failFast bool) <-chan StreamEvent {
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

			wg.Add(1)
			sem <- struct{}{}

			go func(rh *inventory.ResolvedHost) {
				defer wg.Done()
				defer func() { <-sem }()

				displayName := rh.Host.DisplayName()
				start := time.Now()
				ev := StreamEvent{
					Host:     displayName,
					Done:     true,
					Duration: time.Since(start),
					Type:     "ping",
				}

				if success, line, exitCode := doICMPPing(displayName, rh.Host.Name, count); success || exitCode != -1 {
					ev.Success = success
					ev.Line = line
					ev.ExitCode = exitCode
					if !success {
						failed.Store(true)
					}
					out <- ev
					return
				}

				if success, line, exitCode, dur := doTCPPing(displayName, rh.Host.Name, count, time.Since(start)); true {
					ev.Success = success
					ev.Line = line
					ev.ExitCode = exitCode
					ev.Duration = dur
					if !success {
						failed.Store(true)
					}
					out <- ev
					return
				}
			}(hosts[i])
		}

		wg.Wait()
	}()

	return out
}

func doICMPPing(displayName, host string, count int) (bool, string, int) {
	pinger, err := ping.NewPinger(host)
	if err != nil {
		return false, err.Error(), -1
	}

	pinger.Count = count
	pinger.Timeout = time.Duration(count)*time.Second + 5*time.Second
	pinger.SetPrivileged(false)

	err = pinger.Run()
	stats := pinger.Statistics()

	if err != nil {
		if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "socket: permission denied") {
			return false, "", -1
		}
		return false, err.Error(), -1
	}

	if stats.PacketsRecv == 0 {
		return false, "100% packet loss", 1
	}

	minMs := float64(stats.MinRtt) / 1e6
	avgMs := float64(stats.AvgRtt) / 1e6
	maxMs := float64(stats.MaxRtt) / 1e6
	ok := stats.PacketsRecv
	failed := stats.PacketsSent - stats.PacketsRecv
	return true, fmt.Sprintf("min=%.3fms avg=%.3fms max=%.3fms ok=%d failed=%d total=%d", minMs, avgMs, maxMs, ok, failed, stats.PacketsSent), 0
}

func doTCPPing(displayName, host string, count int, elapsed time.Duration) (bool, string, int, time.Duration) {
	ports := []int{22, 80, 443, 8080}
	var times []time.Duration

	for i := 0; i < count; i++ {
		for _, port := range ports {
			start := time.Now()
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 5*time.Second)
			dur := time.Since(start)
			if err == nil {
				conn.Close()
				times = append(times, dur)
				break
			}
		}
		if len(times) == 0 || times[len(times)-1] != times[0] {
			time.Sleep(100 * time.Millisecond)
		}
	}

	if len(times) == 0 {
		return false, "tcp ping failed (no open ports detected)", 1, elapsed
	}

	var minMs, avgMs, maxMs float64
	minMs = float64(times[0].Nanoseconds()) / 1e6
	maxMs = minMs
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
	avgMs = sum / float64(len(times))

	elapsed += times[len(times)-1]
	ok := len(times)
	failed := count - len(times)
	return true, fmt.Sprintf("min=%.3fms avg=%.3fms max=%.3fms ok=%d failed=%d total=%d", minMs, avgMs, maxMs, ok, failed, count), 0, elapsed
}
