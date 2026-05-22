// Copyright 2025 Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	golog "log"

	"github.com/daytonaio/common-go/pkg/log"
	"github.com/daytonaio/daemon/cmd/daemon/config"
	"github.com/daytonaio/daemon/pkg/childreap"
	"github.com/daytonaio/daemon/pkg/recording"
	"github.com/daytonaio/daemon/pkg/recordingdashboard"
	"github.com/daytonaio/daemon/pkg/session"
	"github.com/daytonaio/daemon/pkg/ssh"
	"github.com/daytonaio/daemon/pkg/terminal"
	"github.com/daytonaio/daemon/pkg/toolbox"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"gopkg.in/natefinch/lumberjack.v2"
)

func main() {
	os.Exit(run())
}

func run() int {
	logLevel := log.ParseLogLevel(os.Getenv("LOG_LEVEL"))

	// Create the console handler with tint for colored output
	consoleHandler := tint.NewHandler(os.Stdout, &tint.Options{
		NoColor:    !isatty.IsTerminal(os.Stdout.Fd()),
		TimeFormat: time.RFC3339,
		Level:      logLevel,
	})

	logger := slog.New(consoleHandler)
	slog.SetDefault(logger)

	// Redirect standard library log to slog
	golog.SetOutput(&log.DebugLogWriter{})

	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Error("Failed to get user home directory", "error", err)
		return 2
	}

	configDir := filepath.Join(homeDir, ".daytona")
	err = os.MkdirAll(configDir, 0755)
	if err != nil {
		logger.Error("Failed to create config directory", "path", configDir, "error", err)
		return 2
	}

	entrypointLogFilePath := getEntrypointLogFilePath(configDir)

	args := os.Args[1:]
	if exitCode, handled := handlePlatformSubcommands(args, entrypointLogFilePath, logger); handled {
		return exitCode
	}

	c, err := config.GetConfig()
	if err != nil {
		logger.Error("Failed to get config", "error", err)
		return 2
	}

	platformInit(logger)

	// If workdir in image is not set, use user home as workdir
	if c.UserHomeAsWorkDir {
		err = os.Chdir(homeDir)
		if err != nil {
			logger.Warn("Failed to change working directory to home directory", "error", err)
		}
	}

	if c.DaemonLogFilePath != "" {
		_ = os.MkdirAll(filepath.Dir(c.DaemonLogFilePath), 0755)
		logFile, err := os.OpenFile(c.DaemonLogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			logger.Error("Failed to open daemon log file", "path", c.DaemonLogFilePath, "error", err)
		} else {
			_ = logFile.Close()

			logWriter := &lumberjack.Logger{
				Filename:   c.DaemonLogFilePath,
				MaxSize:    c.DaemonLogMaxSizeMB,
				MaxAge:     c.DaemonLogMaxAgeDays,
				MaxBackups: c.DaemonLogMaxBackups,
				LocalTime:  true,
				Compress:   c.DaemonLogCompress,
			}
			defer logWriter.Close()

			fileHandler := slog.NewTextHandler(logWriter, &slog.HandlerOptions{
				Level: logLevel,
			})
			handler := log.NewMultiHandler([]slog.Handler{consoleHandler, fileHandler}...)

			logger = slog.New(handler)
			slog.SetDefault(logger)
		}
	}

	sessionService := session.NewSessionService(logger, configDir, c.TerminationGracePeriod, c.TerminationCheckInterval)

	entrypointCleanup := setupEntrypoint(args, sessionService, logger)
	if len(args) > 0 && entrypointCleanup == nil {
		return 2
	}
	if entrypointCleanup != nil {
		defer entrypointCleanup()
	}

	errChan := make(chan error)

	workDir, err := os.Getwd()
	if err != nil {
		logger.Error("Failed to get current working directory", "error", err)
		return 2
	}

	recordingsDir := c.RecordingsDir
	if recordingsDir == "" {
		recordingsDir = filepath.Join(configDir, "recordings")
	}
	recordingService := recording.NewRecordingService(logger, recordingsDir)

	toolBoxServer := toolbox.NewServer(toolbox.ServerConfig{
		Logger:                logger,
		WorkDir:               workDir,
		ConfigDir:             configDir,
		OtelEndpoint:          c.OtelEndpoint,
		SandboxId:             c.SandboxId,
		SessionService:        sessionService,
		RecordingService:      recordingService,
		OrganizationId:        c.OrganizationId,
		RegionId:              c.RegionId,
		Snapshot:              c.Snapshot,
		EntrypointLogFilePath: entrypointLogFilePath,
	})

	// Start the toolbox server in a go routine
	go func() {
		err := toolBoxServer.Start()
		if err != nil {
			errChan <- err
		}
	}()

	// Start terminal server
	go func() {
		if err := terminal.StartTerminalServer(22222); err != nil {
			errChan <- err
		}
	}()

	// Start recording dashboard server
	go func() {
		if err := recordingdashboard.NewDashboardServer(logger, recordingService).Start(); err != nil {
			errChan <- err
		}
	}()

	sshServer := ssh.NewServer(logger, workDir, workDir)

	go func() {
		if err := sshServer.Start(); err != nil {
			errChan <- err
		}
	}()

	// Reap zombie children. The daemon runs as PID 1 inside containers, so
	// orphaned processes (e.g. from process.exec) get reparented here.
	// childreap installs the SIGCHLD reaper AND wires up cooperative status
	// recovery so cmd.Wait callers still get the right exit code when the
	// reaper claims a child before they do (see pkg/childreap).
	childreap.Start()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	registerSignals(sigChan)

	// Wait for either an error or shutdown signal
	select {
	case err := <-errChan:
		logger.Error("Error occurred", "error", err)
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down gracefully...", "signal", sig)
	}

	// Toolbox server graceful shutdown
	toolBoxServer.Shutdown()

	slog.Info("Shutdown complete")
	return 0
}
