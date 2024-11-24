// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"
	"tailscale.com/util/lineread"
)

const tsConfigStartMark = "## BEGIN Tailscale ##"
const tsConfigEndMark = "## END Tailscale ##"

func init() {
	configureCmd.Subcommands = append(configureCmd.Subcommands, configureSSHconfigCmd)
}

var configureSSHconfigCmd = &ffcli.Command{
	Name:       "sshconfig",
	ShortHelp:  "[ALPHA] Configure $HOME/.ssh/config to check Tailscale for KnownHosts",
	ShortUsage: "sshconfig >> $HOME/.ssh/config",
	LongHelp: strings.TrimSpace(`
Run this command to output a ssh_config snippet that configures SSH to check
Tailscale for KnownHosts.

You can use this snippet by running: tailscale sshconfig >> $HOME/.ssh/config
or copy and paste it into your $HOME/.ssh/config file.
`),
	Exec: runConfigureSSHconfig,
	FlagSet: (func() *flag.FlagSet {
		fs := newFlagSet("sshconfig")
		fs.BoolVar(&sshConfigArgs.export, "export", false, "export the config snippet to stdout or modify $HOME/.ssh/config in place")
		return fs
	})(),
}

var sshConfigArgs struct {
	export bool // export the config snippet to stdout or modify in place
}

func replaceBetween[S ~[]T, T any](s S, start, end int, replacement []T) S {
	if start < 0 || end < 0 || start > end || end > len(s) {
		panic("invalid indices")
	}
	if start == end {
		return s
	}
	return append(append(s[:start+1:start+1], replacement...), s[end:]...)
}

// runConfigureSSHconfig updates the user's $HOME/.ssh/config file to add the
// Tailscale config snippet. If the snippet is not present, it will be appended
// between the BEGIN and END marks. If it is present it will be updated if needed.
func runConfigureSSHconfig(ctx context.Context, args []string) error {
	if len(args) > 0 {
		return errors.New("unexpected non-flag arguments to 'tailscale status'")
	}
	tailscaleBin, err := os.Executable()
	if err != nil {
		return err
	}
	st, err := localClient.Status(ctx)
	if err != nil {
		return err
	}

	tsSshConfig, err := genSSHConfig(st, tailscaleBin)
	if err != nil {
		return err
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	if !sshConfigArgs.export {
		sshConfigFilePath := filepath.FromSlash(h + "/.ssh/config")
		fmt.Println(sshConfigFilePath)
		var sshConfig []string

		// Create the file if it does not exist
		_, err = os.OpenFile(sshConfigFilePath, os.O_RDONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}

		err = lineread.File(sshConfigFilePath, func(line []byte) error {
			sshConfig = append(sshConfig, string(line))
			return nil
		})
		if err != nil {
			return err
		}

		start, end := findConfigMark(sshConfig)
		if start == -1 || end == -1 {
			sshConfig = append(sshConfig, tsConfigStartMark)
			sshConfig = append(sshConfig, tsSshConfig)
			sshConfig = append(sshConfig, tsConfigEndMark)
		} else {
			existingConfig := strings.Join(sshConfig[start+1:end], "\n")
			if existingConfig != tsSshConfig {
				sshConfig = replaceBetween(sshConfig, start+1, end, []string{tsSshConfig})
			}
		}

		sshFile, err := os.Create(sshConfigFilePath)
		if err != nil {
			return err

		}
		defer sshFile.Close()

		for _, line := range sshConfig {
			_, err := sshFile.WriteString(line + "\n")
			if err != nil {
				return err
			}
		}
		fmt.Printf("Updated %s\n", sshConfigFilePath)
	} else {
		fmt.Println(tsSshConfig)
	}

	return nil
}

// findConfigMark finds and returns the index of the tsConfigStartMark and
// tsConfigEndmark in a file. If the file doesn't contain the marks, it returns
// -1, -1
func findConfigMark(file []string) (int, int) {
	start := -1
	end := -1
	for i, v := range file {
		if strings.Contains(v, tsConfigStartMark) {
			start = i
		}
		if strings.Contains(v, tsConfigEndMark) {
			end = i
		}
	}
	return start, end
}
