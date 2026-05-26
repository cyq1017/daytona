// Copyright 2025 Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package otel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

func newConsoleHandler(level slog.Level) slog.Handler {
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		return tint.NewHandler(os.Stdout, &tint.Options{
			Level:      level,
			TimeFormat: time.RFC3339,
		})
	}
	return slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
}

func initConsoleLogging(level slog.Level) {
	slog.SetDefault(slog.New(newConsoleHandler(level)))
}

// InitConsoleLogging installs a console-only slog handler as the default logger.
// Useful for emitting log lines before the full Init() call (e.g. while loading
// the runner config). The full Init() will reinstall the same handler and
// optionally fan out to OTel.
func InitConsoleLogging(level slog.Level) {
	initConsoleLogging(level)
}

func initOTelLogging(ctx context.Context, res *resource.Resource, serviceName string, level slog.Level) (func(context.Context) error, error) {
	exp, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return noop, fmt.Errorf("otel: create log exporter: %w", err)
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(lp)

	consoleHandler := newConsoleHandler(level)
	otelHandler := otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(lp))

	slog.SetDefault(slog.New(&fanoutHandler{handlers: []slog.Handler{consoleHandler, otelHandler}}))

	return lp.Shutdown, nil
}

type fanoutHandler struct {
	handlers []slog.Handler
}

func (f *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f.handlers {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		hs[i] = h.WithAttrs(attrs)
	}
	return &fanoutHandler{handlers: hs}
}

func (f *fanoutHandler) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		hs[i] = h.WithGroup(name)
	}
	return &fanoutHandler{handlers: hs}
}
