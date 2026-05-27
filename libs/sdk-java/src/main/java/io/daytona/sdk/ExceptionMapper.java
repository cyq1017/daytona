// Copyright Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package io.daytona.sdk;

import io.daytona.sdk.exception.DaytonaAuthenticationException;
import io.daytona.sdk.exception.DaytonaBadRequestException;
import io.daytona.sdk.exception.DaytonaConflictException;
import io.daytona.sdk.exception.DaytonaConnectionException;
import io.daytona.sdk.exception.DaytonaException;
import io.daytona.sdk.exception.DaytonaForbiddenException;
import io.daytona.sdk.exception.DaytonaNotFoundException;
import io.daytona.sdk.exception.DaytonaRateLimitException;
import io.daytona.sdk.exception.DaytonaServerException;
import io.daytona.sdk.exception.DaytonaTimeoutException;
import io.daytona.sdk.exception.DaytonaValidationException;

import java.net.SocketTimeoutException;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

final class ExceptionMapper {
    private ExceptionMapper() {
    }

    static <T> T callMain(MainSupplier<T> supplier) {
        try {
            return supplier.get();
        } catch (io.daytona.api.client.ApiException e) {
            throw map(e.getCode(), e.getResponseBody(), flattenHeaders(e.getResponseHeaders()), e);
        }
    }

    static void runMain(MainRunnable runnable) {
        try {
            runnable.run();
        } catch (io.daytona.api.client.ApiException e) {
            throw map(e.getCode(), e.getResponseBody(), flattenHeaders(e.getResponseHeaders()), e);
        }
    }

    static <T> T callToolbox(ToolboxSupplier<T> supplier) {
        try {
            return supplier.get();
        } catch (io.daytona.toolbox.client.ApiException e) {
            throw map(e.getCode(), e.getResponseBody(), flattenHeaders(e.getResponseHeaders()), e);
        }
    }

    static void runToolbox(ToolboxRunnable runnable) {
        try {
            runnable.run();
        } catch (io.daytona.toolbox.client.ApiException e) {
            throw map(e.getCode(), e.getResponseBody(), flattenHeaders(e.getResponseHeaders()), e);
        }
    }

    static DaytonaException map(int statusCode, String responseBody, Throwable cause) {
        return map(statusCode, responseBody, Collections.emptyMap(), cause);
    }

    static DaytonaException map(int statusCode, String responseBody, Map<String, String> headers, Throwable cause) {
        // Only treat status==0 as a transport failure when the ApiException
        // wraps an underlying Throwable; client-side ApiExceptions thrown for
        // parameter validation also have status==0 but no wrapped cause.
        if (statusCode == 0 && (responseBody == null || responseBody.isEmpty())
                && cause != null && cause.getCause() != null) {
            return mapTransportFailure(cause);
        }
        ErrorDetails errorDetails = extractErrorDetails(responseBody, statusCode);
        String message = errorDetails.message();
        if (statusCode == 0 && (responseBody == null || responseBody.isEmpty())
                && cause != null && cause.getMessage() != null && !cause.getMessage().isEmpty()) {
            message = cause.getMessage();
        }
        switch (statusCode) {
            case 400:
                return new DaytonaBadRequestException(message, cause, errorDetails.code(), errorDetails.source());
            case 401:
                return new DaytonaAuthenticationException(message, cause, errorDetails.code(), errorDetails.source());
            case 403:
                return new DaytonaForbiddenException(message, cause, errorDetails.code(), errorDetails.source());
            case 404:
                return new DaytonaNotFoundException(message, cause, errorDetails.code(), errorDetails.source());
            case 409:
                return new DaytonaConflictException(message, cause, errorDetails.code(), errorDetails.source());
            case 422:
                return new DaytonaValidationException(message, cause, errorDetails.code(), errorDetails.source());
            case 429:
                return new DaytonaRateLimitException(message, cause, errorDetails.code(), errorDetails.source());
            default:
                if (statusCode >= 500) {
                    return new DaytonaServerException(statusCode, message, cause, errorDetails.code(), errorDetails.source());
                }
                return new DaytonaException(statusCode, message, headers, cause, errorDetails.code(), errorDetails.source());
        }
    }

    private static DaytonaException mapTransportFailure(Throwable cause) {
        Throwable root = rootCause(cause);
        String message = rootMessage(root);
        if (root instanceof SocketTimeoutException) {
            return new DaytonaTimeoutException("Request timed out: " + message, cause);
        }
        return new DaytonaConnectionException("Connection failed: " + message, cause);
    }

    private static Throwable rootCause(Throwable t) {
        Throwable current = t;
        while (current.getCause() != null && current.getCause() != current) {
            current = current.getCause();
        }
        return current;
    }

    private static String rootMessage(Throwable t) {
        String msg = t.getMessage();
        if (msg != null && !msg.isEmpty()) {
            return msg;
        }
        return t.getClass().getSimpleName();
    }

    /**
     * Extracts a human-readable message from a raw JSON response body.
     * Looks for a "message" or "error" field; falls back to the raw body or a generic message.
     */
    private static ErrorDetails extractErrorDetails(String responseBody, int statusCode) {
        if (responseBody == null || responseBody.isEmpty()) {
            return new ErrorDetails("Request failed with status " + statusCode, null, null);
        }

        String message = extractJsonField(responseBody, "message");
        if (message == null) {
            message = extractJsonField(responseBody, "error");
        }
        if (message == null) {
            message = responseBody;
        }

        return new ErrorDetails(
                message,
                extractJsonField(responseBody, "code"),
                extractJsonField(responseBody, "source"));
    }

    private static String extractJsonField(String responseBody, String field) {
        Matcher matcher = Pattern.compile("\"" + Pattern.quote(field) + "\"\\s*:\\s*\"((?:[^\"\\\\]|\\\\.)*)\"")
                .matcher(responseBody);
        if (matcher.find()) {
            return matcher.group(1);
        }
        return null;
    }

    private static Map<String, String> flattenHeaders(Map<String, List<String>> responseHeaders) {
        if (responseHeaders == null || responseHeaders.isEmpty()) {
            return Collections.emptyMap();
        }

        Map<String, String> flattenedHeaders = new LinkedHashMap<>();
        for (Map.Entry<String, List<String>> entry : responseHeaders.entrySet()) {
            List<String> values = entry.getValue();
            flattenedHeaders.put(entry.getKey(), values == null ? "" : String.join(", ", values));
        }
        return flattenedHeaders;
    }

    private static final class ErrorDetails {
        private final String message;
        private final String code;
        private final String source;

        private ErrorDetails(String message, String code, String source) {
            this.message = message;
            this.code = code;
            this.source = source;
        }

        private String message() {
            return message;
        }

        private String code() {
            return code;
        }

        private String source() {
            return source;
        }
    }

    @FunctionalInterface
    interface MainSupplier<T> {
        T get() throws io.daytona.api.client.ApiException;
    }

    @FunctionalInterface
    interface MainRunnable {
        void run() throws io.daytona.api.client.ApiException;
    }

    @FunctionalInterface
    interface ToolboxSupplier<T> {
        T get() throws io.daytona.toolbox.client.ApiException;
    }

    @FunctionalInterface
    interface ToolboxRunnable {
        void run() throws io.daytona.toolbox.client.ApiException;
    }
}
