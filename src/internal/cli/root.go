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

package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dhoard/fleetsh/internal/inventory"
	"github.com/dhoard/fleetsh/internal/output"
	"github.com/dhoard/fleetsh/internal/sshrun"
)

const (
	minParallel = 1
	minTimeout  = 1
	defaultPing = 3
)

var (
	flagInventory string
	flagGroup     string
	flagCommand   string
	flagScript    string
	flagPing      int
	flagParallel  int
	flagTimeout   int
	flagDryRun    bool
	flagJSON      bool
	flagFailFast  bool
)

func Execute() {
	rootCmd := buildRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(2)
	}
}

func buildRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "fleetsh [alias] [flags]",
		Short:   "fleetsh v" + version,
		Long:    "fleetsh v" + version,
		Args:    cobra.MaximumNArgs(1),
		RunE:    runE,
		Version: version,
	}

	cmd.SetVersionTemplate("fleetsh v{{.Version}}\n")
	cmd.Flags().SortFlags = false
	cmd.Flags().StringVarP(&flagGroup, "group", "g", "", "group to target")
	cmd.Flags().StringVarP(&flagInventory, "inventory", "i", "", "inventory file path (default: .fleetsh, then ~/.fleetsh)")
	cmd.Flags().StringVarP(&flagCommand, "command", "c", "", "command to run remotely (mutually exclusive with -s, -p)")
	cmd.Flags().StringVarP(&flagScript, "script", "s", "", "local script file to execute remotely (mutually exclusive with -c, -p)")
	cmd.Flags().IntVarP(&flagTimeout, "timeout", "t", 30000, "per-host timeout in milliseconds")
	cmd.Flags().IntVarP(&flagParallel, "parallel", "l", 1, "max concurrent hosts (default: sequential)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "output JSON")
	cmd.Flags().BoolVar(&flagFailFast, "fail-fast", false, "stop scheduling new hosts after first failure")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "print what would run without connecting")
	cmd.Flags().IntVarP(&flagPing, "ping", "p", 0, "ping hosts (default: 3) (mutually exclusive with -c, -s)")
	cmd.Flags().Bool("help", false, "help for fleetsh")

	return cmd
}

func runE(cmd *cobra.Command, args []string) error {
	errf := func(format string, args ...interface{}) error {
		return fmt.Errorf("\n  "+format+"\n", args...)
	}

	if len(args) == 0 && flagGroup == "" {
		if flagCommand == "" && flagScript == "" && flagPing == 0 {
			cmd.Help()
			return nil
		}
	}

	if len(args) > 0 && flagGroup != "" {
		return errf("cannot specify both an alias argument and --group")
	}

	if cmd.Flags().Changed("ping") && flagPing < 1 {
		return errf("--ping must be >= 1, got %d", flagPing)
	}

	if flagCommand != "" && flagScript != "" {
		return errf("--command and --script are mutually exclusive")
	}

	if flagPing > 0 && (flagCommand != "" || flagScript != "") {
		return errf("--ping is mutually exclusive with --command and --script")
	}

	if flagCommand == "" && flagScript == "" && flagPing == 0 {
		return errf("exactly one of --command, --script, or --ping is required")
	}

	pingCount := flagPing
	if pingCount == 0 {
		pingCount = defaultPing
	}

	if flagParallel < minParallel {
		return errf("--parallel must be >= %d, got %d", minParallel, flagParallel)
	}
	if flagTimeout < minTimeout {
		return errf("--timeout must be >= %d, got %d", minTimeout, flagTimeout)
	}
	if flagInventory != "" {
		if _, err := os.Stat(flagInventory); err != nil {
			return errf("cannot access inventory file %q: %w", flagInventory, err)
		}
	}

	if flagPing <= 0 && !flagDryRun {
		if _, err := exec.LookPath("ssh"); err != nil {
			return errf("ssh not found: please install OpenSSH client and ensure ssh is on your PATH")
		}
		if _, err := exec.LookPath("scp"); err != nil {
			return errf("scp not found: please install OpenSSH client and ensure scp is on your PATH")
		}
	}

	inventoryPath := flagInventory
	if inventoryPath == "" {
		inventoryPath = resolveDefaultInventory()
	}

	inv, err := inventory.Parse(inventoryPath)
	if err != nil {
		return err
	}

	target := flagGroup
	if target == "" {
		target = args[0]
	}

	hosts, err := inv.Resolve(target)
	if err != nil {
		return err
	}

	if len(hosts) == 0 {
		return fmt.Errorf("no hosts found for target %q", target)
	}

	maxLen := 7
	for _, h := range hosts {
		nameLen := len(h.Host.DisplayName())
		if nameLen > maxLen {
			maxLen = nameLen
		}
	}

	versionMsg := fmt.Sprintf("fleetsh v%s", version)
	warningMsg := ""

	var scriptContent []byte
	isScript := false
	command := flagCommand

	if flagScript != "" {
		isScript = true
		scriptContent, err = os.ReadFile(flagScript)
		if err != nil {
			return fmt.Errorf("cannot read script file %q: %w", flagScript, err)
		}
	}

	tasks := sshrun.BuildTasks(hosts, command, scriptContent, time.Duration(flagTimeout)*time.Millisecond, isScript)

	start := time.Now()

	if flagPing > 0 {
		events := sshrun.PingHosts(hosts, pingCount, flagParallel, flagFailFast)
		var results []*sshrun.Result
		if flagJSON {
			results = output.StreamJSON(os.Stdout, versionMsg, warningMsg, events, start)
		} else {
			results = output.StreamText(os.Stdout, versionMsg, warningMsg, events, maxLen, start)
		}
		summary := sshrun.ComputeSummary(results)
		if summary.Failed > 0 {
			os.Exit(1)
		}
		return nil
	}

	runner := sshrun.NewRunner(flagParallel, flagFailFast, flagDryRun)
	events := runner.Stream(context.Background(), tasks)

	var results []*sshrun.Result
	if flagJSON {
		results = output.StreamJSON(os.Stdout, versionMsg, warningMsg, events, start)
	} else {
		results = output.StreamText(os.Stdout, versionMsg, warningMsg, events, maxLen, start)
	}

	summary := sshrun.ComputeSummary(results)

	if summary.Failed > 0 {
		os.Exit(1)
	}

	return nil
}

func alignPrefix(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func resolveDefaultInventory() string {
	cwd, err := os.Getwd()
	if err == nil {
		local := filepath.Join(cwd, ".fleetsh")
		if _, err := os.Stat(local); err == nil {
			return local
		}
	}

	home, err := os.UserHomeDir()
	if err == nil {
		global := filepath.Join(home, ".fleetsh")
		if _, err := os.Stat(global); err == nil {
			return global
		}
	}

	return ".fleetsh"
}
