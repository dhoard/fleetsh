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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// exitError carries a specific process exit code from runE up to Execute,
// which is the single place that maps errors to os.Exit codes. This keeps the
// exit-code policy centralized rather than scattering os.Exit calls through the
// command logic.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

// Execute runs the root command and maps its result to a process exit code:
//
//	0 — success
//	1 — one or more hosts failed (carried via *exitError)
//	2 — CLI, config, or inventory error (any other non-nil error)
func Execute() {
	rootCmd := buildRootCmd()
	err := rootCmd.Execute()
	if err == nil {
		return
	}

	var ee *exitError
	if errors.As(err, &ee) {
		os.Exit(ee.code)
	}
	os.Exit(2)
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
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		if strings.Contains(err.Error(), "flag needs an argument") {
			if conflict := mutuallyExclusiveConflict(os.Args[1:]); conflict != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Error:\n  %s\n\n", conflict)
				cmd.Usage()
				return fmt.Errorf("%s", conflict)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Error:\n  %s\n\n", err.Error())
		cmd.Usage()
		return err
	})
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
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(cmd.OutOrStdout(), "Error:\n  %s\n\n", msg)
		cmd.Usage()
		return fmt.Errorf("%s", msg)
	}

	errp := func(err error) error {
		fmt.Fprintf(cmd.OutOrStdout(), "Error:\n  %s\n", err.Error())
		return err
	}

	if len(args) == 0 && flagGroup == "" {
		if flagCommand == "" && flagScript == "" && !cmd.Flags().Changed("ping") {
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

	if (flagCommand != "" && flagScript != "") || (flagPing > 0 && (flagCommand != "" || flagScript != "")) {
		if conflict := mutuallyExclusiveConflict(os.Args[1:]); conflict != "" {
			return errf("%s", conflict)
		}
		return errf("--command, --script, and --ping are mutually exclusive")
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
			return errf("cannot access inventory file %q: %v", flagInventory, err)
		}
	}

	if flagPing == 0 && !flagDryRun {
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
		return errp(err)
	}

	target := flagGroup
	if target == "" {
		if len(args) > 0 {
			target = args[0]
		}
	}

	target = strings.TrimSpace(target)

	hosts, err := resolveWithPattern(inv, target, flagGroup != "")
	if err != nil {
		return errp(err)
	}

	if len(hosts) == 0 {
		return errp(fmt.Errorf("no hosts found for target %q", target))
	}

	maxLen := 7
	for _, h := range hosts {
		nameLen := len([]rune(h.Host.DisplayName()))
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
			return errp(fmt.Errorf("cannot read script file %q: %w", flagScript, err))
		}
	}

	tasks := sshrun.BuildTasks(hosts, command, scriptContent, time.Duration(flagTimeout)*time.Millisecond, isScript)

	start := time.Now()

	if flagPing > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(flagTimeout)*time.Millisecond)
		defer cancel()
		events := sshrun.PingHosts(ctx, hosts, pingCount, flagParallel, flagFailFast)
		var results []*sshrun.Result
		if flagJSON {
			results = output.StreamJSON(os.Stdout, versionMsg, warningMsg, events, start)
		} else {
			results = output.StreamText(os.Stdout, versionMsg, warningMsg, events, maxLen, start)
		}
		summary := sshrun.ComputeSummary(results)
		if summary.Failed > 0 {
			return &exitError{code: 1}
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
		return &exitError{code: 1}
	}

	return nil
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

func isPattern(target string) bool {
	return strings.HasPrefix(target, "[") && strings.HasSuffix(target, "]")
}

// mutuallyExclusiveConflict scans the command-line arguments for the mutually
// exclusive flags --command/-c, --script/-s, and --ping/-p. If two or more are
// present, it returns an error message listing them in the order they were
// provided on the command line. Otherwise it returns an empty string.
func mutuallyExclusiveConflict(args []string) string {
	type flagName struct {
		long  string
		short string
		name  string
	}
	exclusive := []flagName{
		{"--command", "-c", "--command"},
		{"--script", "-s", "--script"},
		{"--ping", "-p", "--ping"},
	}

	var seen []string
	for _, arg := range args {
		for _, f := range exclusive {
			if arg == f.long || arg == f.short ||
				strings.HasPrefix(arg, f.long+"=") || strings.HasPrefix(arg, f.short+"=") {
				already := false
				for _, s := range seen {
					if s == f.name {
						already = true
						break
					}
				}
				if !already {
					seen = append(seen, f.name)
				}
			}
		}
	}

	if len(seen) < 2 {
		return ""
	}

	return strings.Join(seen, ", ") + " are mutually exclusive"
}

func extractPattern(target string) (string, error) {
	if !isPattern(target) {
		return "", nil
	}
	inner := strings.TrimSpace(target[1 : len(target)-1])
	if inner == "" {
		return "", fmt.Errorf("empty pattern")
	}
	return inner, nil
}

func resolveWithPattern(inv *inventory.Inventory, target string, isGroupFlag bool) ([]*inventory.ResolvedHost, error) {
	patternStr, err := extractPattern(target)
	if err != nil {
		return nil, err
	}

	if patternStr != "" {
		re, err := regexp.Compile(patternStr)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", patternStr, err)
		}
		if isGroupFlag {
			return inv.ResolveGroupPattern(re)
		}
		return inv.ResolveAliasPattern(re)
	}

	hosts, err := inv.Resolve(target)
	if err == nil && len(hosts) > 0 {
		return hosts, nil
	}

	if isGroupFlag {
		re, reErr := regexp.Compile(target)
		if reErr == nil {
			hosts, err = inv.ResolveGroupPattern(re)
			if err == nil {
				return hosts, nil
			}
		}
	}

	return nil, fmt.Errorf("no hosts found for target %q", target)
}
