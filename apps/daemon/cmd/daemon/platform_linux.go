//go:build linux

// Copyright Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/daytonaio/daemon/internal/util"
	"github.com/daytonaio/daemon/pkg/session"
)

func getEntrypointLogFilePath(configDir string) string {
	return filepath.Join(configDir, "sessions", util.EntrypointSessionID, util.EntrypointCommandID, "output.log")
}

func handlePlatformSubcommands(args []string, entrypointLogFilePath string, logger *slog.Logger) (int, bool) {
	if len(args) == 2 && args[0] == "entrypoint" && args[1] == "logs" {
		err := util.ReadEntrypointLogs(entrypointLogFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				logger.Warn("Logs not found, please check if correct entrypoint was provided for sandbox.")
			} else {
				logger.Error("Failed to read entrypoint log file", "error", err)
			}
			return 1, true
		}
		return 0, true
	}

	return 0, false
}

func setupEntrypoint(args []string, sessionService *session.SessionService, logger *slog.Logger) func() {
	if len(args) == 0 {
		return nil
	}

	err := sessionService.Create(util.EntrypointSessionID, false)
	if err != nil {
		logger.Error("Failed to create entrypoint session", "error", err)
		return nil
	}

	logger.Debug("Created entrypoint session", "session_id", util.EntrypointSessionID)

	command := util.ShellQuoteJoin(args)
	_, err = sessionService.Execute(
		util.EntrypointSessionID,
		util.EntrypointCommandID,
		command,
		true,
		false,
		true,
	)
	if err != nil {
		logger.Error("Failed to execute entrypoint command", "error", err)
		return nil
	}

	return func() {
		delErr := sessionService.Delete(context.Background(), util.EntrypointSessionID)
		if delErr != nil {
			logger.Error("Failed to delete entrypoint session", "error", delErr)
		} else {
			logger.Debug("Deleted entrypoint session", "session_id", util.EntrypointSessionID)
		}
	}
}

func platformInit(logger *slog.Logger) {}

func registerSignals(sigChan chan<- os.Signal) {
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
}
