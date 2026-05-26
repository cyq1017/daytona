// Copyright 2025 Daytona Platforms Inc.
// SPDX-License-Identifier: AGPL-3.0

package otel

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

func newResource(ctx context.Context, serviceName, serviceVersion, environment string) (*resource.Resource, error) {
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithHostID(),
		resource.WithOS(),
		resource.WithProcessPID(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			semconv.ServiceNamespace("runner"),
			semconv.DeploymentEnvironmentName(environment),
		),
	)
	if errors.Is(err, resource.ErrPartialResource) || errors.Is(err, resource.ErrSchemaURLConflict) {
		return res, nil
	}
	if err != nil {
		return nil, fmt.Errorf("otel: build resource: %w", err)
	}
	return res, nil
}
