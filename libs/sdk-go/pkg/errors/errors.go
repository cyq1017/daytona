// Copyright 2025 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"encoding/json"
	"fmt"
	"net/http"

	apiclient "github.com/daytonaio/daytona/libs/api-client-go"
	"github.com/daytonaio/daytona/libs/toolbox-api-client-go"
)

// DaytonaError is the base error type for all Daytona SDK errors
type DaytonaError struct {
	Message    string
	StatusCode int
	Headers    http.Header
	Code       string
	Source     string
}

func (e *DaytonaError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("Daytona error (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("Daytona error: %s", e.Message)
}

func (e *DaytonaError) base() *DaytonaError { return e }

// NewDaytonaError creates a new DaytonaError
func NewDaytonaError(message string, statusCode int, headers http.Header) *DaytonaError {
	return &DaytonaError{
		Message:    message,
		StatusCode: statusCode,
		Headers:    headers,
	}
}

func parseErrorBody(body []byte) (message, code, source string, parsedStatusCode int) {
	if len(body) == 0 {
		return "", "", "", 0
	}

	var errResp struct {
		Message    string `json:"message"`
		Error      string `json:"error"`
		StatusCode int    `json:"statusCode"`
		Code       string `json:"code"`
		Source     string `json:"source"`
	}

	if json.Unmarshal(body, &errResp) != nil {
		return string(body), "", "", 0
	}

	if errResp.Message != "" {
		message = errResp.Message
	} else if errResp.Error != "" {
		message = errResp.Error
	}

	return message, errResp.Code, errResp.Source, errResp.StatusCode
}

type daytonaErrorCarrier interface {
	base() *DaytonaError
}

func applyErrorMetadata(err error, code, source string) error {
	if carrier, ok := err.(daytonaErrorCarrier); ok {
		b := carrier.base()
		b.Code = code
		b.Source = source
	}
	return err
}

// DaytonaNotFoundError represents a resource not found error (404)
type DaytonaNotFoundError struct {
	*DaytonaError
}

func (e *DaytonaNotFoundError) Error() string {
	return fmt.Sprintf("Resource not found: %s", e.Message)
}

// NewDaytonaNotFoundError creates a new DaytonaNotFoundError
func NewDaytonaNotFoundError(message string, headers http.Header) *DaytonaNotFoundError {
	return &DaytonaNotFoundError{
		DaytonaError: NewDaytonaError(message, http.StatusNotFound, headers),
	}
}

// DaytonaRateLimitError represents a rate limit error (429)
type DaytonaRateLimitError struct {
	*DaytonaError
}

func (e *DaytonaRateLimitError) Error() string {
	return fmt.Sprintf("Rate limit exceeded: %s", e.Message)
}

// NewDaytonaRateLimitError creates a new DaytonaRateLimitError
func NewDaytonaRateLimitError(message string, headers http.Header) *DaytonaRateLimitError {
	return &DaytonaRateLimitError{
		DaytonaError: NewDaytonaError(message, http.StatusTooManyRequests, headers),
	}
}

// DaytonaAuthenticationError represents an authentication error (401)
type DaytonaAuthenticationError struct {
	*DaytonaError
}

func (e *DaytonaAuthenticationError) Error() string {
	return fmt.Sprintf("Authentication failed: %s", e.Message)
}

func NewDaytonaAuthenticationError(message string, headers http.Header) *DaytonaAuthenticationError {
	return &DaytonaAuthenticationError{
		DaytonaError: NewDaytonaError(message, http.StatusUnauthorized, headers),
	}
}

// DaytonaForbiddenError represents a forbidden/authorization error (403)
type DaytonaForbiddenError struct {
	*DaytonaError
}

func (e *DaytonaForbiddenError) Error() string {
	return fmt.Sprintf("Forbidden: %s", e.Message)
}

func NewDaytonaForbiddenError(message string, headers http.Header) *DaytonaForbiddenError {
	return &DaytonaForbiddenError{
		DaytonaError: NewDaytonaError(message, http.StatusForbidden, headers),
	}
}

// DaytonaConflictError represents a conflict error (409)
type DaytonaConflictError struct {
	*DaytonaError
}

func (e *DaytonaConflictError) Error() string {
	return fmt.Sprintf("Conflict: %s", e.Message)
}

func NewDaytonaConflictError(message string, headers http.Header) *DaytonaConflictError {
	return &DaytonaConflictError{
		DaytonaError: NewDaytonaError(message, http.StatusConflict, headers),
	}
}

// DaytonaValidationError represents a validation/bad request error (400)
type DaytonaValidationError struct {
	*DaytonaError
}

func (e *DaytonaValidationError) Error() string {
	return fmt.Sprintf("Validation error: %s", e.Message)
}

func NewDaytonaValidationError(message string, headers http.Header) *DaytonaValidationError {
	return &DaytonaValidationError{
		DaytonaError: NewDaytonaError(message, http.StatusBadRequest, headers),
	}
}

// DaytonaServerError represents a server error (5xx)
type DaytonaServerError struct {
	*DaytonaError
}

func (e *DaytonaServerError) Error() string {
	return fmt.Sprintf("Server error: %s", e.Message)
}

func NewDaytonaServerError(message string, statusCode int, headers http.Header) *DaytonaServerError {
	return &DaytonaServerError{
		DaytonaError: NewDaytonaError(message, statusCode, headers),
	}
}

// DaytonaTimeoutError represents a timeout error
type DaytonaTimeoutError struct {
	*DaytonaError
}

func (e *DaytonaTimeoutError) Error() string {
	return fmt.Sprintf("Operation timed out: %s", e.Message)
}

func NewDaytonaTimeoutError(message string) *DaytonaTimeoutError {
	return &DaytonaTimeoutError{
		DaytonaError: NewDaytonaError(message, 0, nil),
	}
}

// NewDaytonaErrorFromBody parses a JSON response body and maps the status code
// to the appropriate SDK error type. Falls back to the raw body as the message.
func NewDaytonaErrorFromBody(body []byte, statusCode int, headers http.Header) error {
	message, code, source, parsedStatusCode := parseErrorBody(body)
	if parsedStatusCode != 0 {
		statusCode = parsedStatusCode
	}

	if message == "" {
		message = "Download failed"
	}

	switch statusCode {
	case http.StatusNotFound:
		return applyErrorMetadata(NewDaytonaNotFoundError(message, headers), code, source)
	case http.StatusTooManyRequests:
		return applyErrorMetadata(NewDaytonaRateLimitError(message, headers), code, source)
	default:
		return applyErrorMetadata(NewDaytonaError(message, statusCode, headers), code, source)
	}
}

// ConvertAPIError converts api-client-go errors to SDK error types
func ConvertAPIError(err error, httpResp *http.Response) error {
	if err == nil {
		return nil
	}

	var message string
	var statusCode int
	var headers http.Header

	if httpResp != nil {
		statusCode = httpResp.StatusCode
		headers = httpResp.Header
	}

	// Try to extract message from GenericOpenAPIError
	if genErr, ok := err.(*apiclient.GenericOpenAPIError); ok {
		body := genErr.Body()
		if len(body) > 0 {
			message, _, _, _ = parseErrorBody(body)
		}

		// Fall back to error string if no body
		if message == "" {
			message = genErr.Error()
		}
	} else {
		message = err.Error()
	}

	code, source := extractErrorMetadata(err)
	return mapStatusCodeToError(statusCode, message, headers, code, source)
}

// ConvertToolboxError converts toolbox-api-client-go errors to SDK error types
func ConvertToolboxError(err error, httpResp *http.Response) error {
	if err == nil {
		return nil
	}

	var message string
	var statusCode int
	var headers http.Header

	if httpResp != nil {
		statusCode = httpResp.StatusCode
		headers = httpResp.Header
	}

	// Try to extract message from GenericOpenAPIError
	if genErr, ok := err.(*toolbox.GenericOpenAPIError); ok {
		body := genErr.Body()
		if len(body) > 0 {
			message, _, _, _ = parseErrorBody(body)
		}

		// Fall back to error string if no body
		if message == "" {
			message = genErr.Error()
		}
	} else {
		message = err.Error()
	}

	code, source := extractErrorMetadata(err)
	return mapStatusCodeToError(statusCode, message, headers, code, source)
}

func extractErrorMetadata(err error) (code, source string) {
	switch genErr := err.(type) {
	case *apiclient.GenericOpenAPIError:
		_, code, source, _ = parseErrorBody(genErr.Body())
	case *toolbox.GenericOpenAPIError:
		_, code, source, _ = parseErrorBody(genErr.Body())
	}

	return code, source
}

func mapStatusCodeToError(statusCode int, message string, headers http.Header, code, source string) error {
	switch {
	case statusCode == http.StatusBadRequest:
		return applyErrorMetadata(NewDaytonaValidationError(message, headers), code, source)
	case statusCode == http.StatusUnauthorized:
		return applyErrorMetadata(NewDaytonaAuthenticationError(message, headers), code, source)
	case statusCode == http.StatusForbidden:
		return applyErrorMetadata(NewDaytonaForbiddenError(message, headers), code, source)
	case statusCode == http.StatusNotFound:
		return applyErrorMetadata(NewDaytonaNotFoundError(message, headers), code, source)
	case statusCode == http.StatusConflict:
		return applyErrorMetadata(NewDaytonaConflictError(message, headers), code, source)
	case statusCode == http.StatusTooManyRequests:
		return applyErrorMetadata(NewDaytonaRateLimitError(message, headers), code, source)
	case statusCode >= 500 && statusCode <= 599:
		return applyErrorMetadata(NewDaytonaServerError(message, statusCode, headers), code, source)
	case statusCode == 0:
		return applyErrorMetadata(NewDaytonaError(message, 0, nil), code, source)
	default:
		return applyErrorMetadata(NewDaytonaError(message, statusCode, headers), code, source)
	}
}
