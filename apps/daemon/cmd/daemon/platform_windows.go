//go:build windows

// Copyright Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"

	"github.com/daytonaio/daemon/pkg/session"
)

func getEntrypointLogFilePath(_ string) string {
	return ""
}

func handlePlatformSubcommands(_ []string, _ string, _ *slog.Logger) (int, bool) {
	return 0, false
}

func setupEntrypoint(args []string, _ *session.SessionService, logger *slog.Logger) func() {
	if len(args) > 0 {
		logger.Warn("Entrypoint args are not supported on Windows; ignoring", "args", args)
		return func() {}
	}

	return nil
}

func platformInit(logger *slog.Logger) {
	ports := []int{2280, 22220, 22222}
	for _, port := range ports {
		cmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
			fmt.Sprintf("name=Daytona Daemon Port %d", port),
			"dir=in", "action=allow", "protocol=tcp",
			fmt.Sprintf("localport=%d", port))
		if err := cmd.Run(); err != nil {
			logger.Warn("Failed to add firewall rule", "port", port, "error", err)
		}
	}
}

func registerSignals(sigChan chan<- os.Signal) {
	signal.Notify(sigChan, os.Interrupt)
}
