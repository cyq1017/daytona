/*
 * Copyright 2025 Daytona Platforms Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * @module Errors
 */

import { AxiosHeaders } from 'axios'
import type { AxiosError } from 'axios'

import { ProxyErrorCode } from '../_generated/proxy-error-code'

export type ResponseHeaders = InstanceType<typeof AxiosHeaders>

/**
 * Base error for Daytona SDK.
 *
 * @example
 * ```ts
 * try {
 *   await daytona.get('missing-sandbox')
 * } catch (error) {
 *   if (error instanceof DaytonaError) {
 *     console.log(error.statusCode)
 *     console.log(error.code)
 *     console.log(error.message)
 *   }
 * }
 * ```
 */
export class DaytonaError extends Error {
  /** HTTP status code if available */
  public statusCode?: number
  /** Machine-readable error code if available */
  public code?: string
  /** Error source if available */
  public readonly source?: string
  /** Response headers if available */
  public headers?: ResponseHeaders

  constructor(message: string, statusCode?: number, headers?: ResponseHeaders, code?: string, source?: string) {
    super(message)
    this.name = new.target.name
    this.statusCode = statusCode
    this.headers = headers
    this.code = code
    this.source = source
  }
}

/**
 * Error thrown when a resource is not found (HTTP 404).
 *
 * @example
 * ```ts
 * try {
 *   await sandbox.fs.downloadFile('/workspace/missing.txt')
 * } catch (error) {
 *   if (error instanceof DaytonaNotFoundError) {
 *     console.log(error.statusCode)
 *   }
 * }
 * ```
 */
export class DaytonaNotFoundError extends DaytonaError {}

/**
 * Error thrown when rate limit is exceeded.
 *
 * @example
 * ```ts
 * try {
 *   for await (const sandbox of daytona.list()) {
 *     console.log(sandbox.id)
 *   }
 * } catch (error) {
 *   if (error instanceof DaytonaRateLimitError) {
 *     console.log(error.code)
 *   }
 * }
 * ```
 */
export class DaytonaRateLimitError extends DaytonaError {}

/**
 * Error thrown when authentication fails (HTTP 401).
 *
 * @example
 * ```ts
 * try {
 *   for await (const sandbox of daytona.list()) {
 *     console.log(sandbox.id)
 *   }
 * } catch (error) {
 *   if (error instanceof DaytonaAuthenticationError) {
 *     console.log(error.statusCode)
 *   }
 * }
 * ```
 */
export class DaytonaAuthenticationError extends DaytonaError {}

/**
 * Error thrown when the request is forbidden (HTTP 403).
 *
 * @example
 * ```ts
 * try {
 *   await daytona.get('sandbox-without-access')
 * } catch (error) {
 *   if (error instanceof DaytonaAuthorizationError) {
 *     console.log(error.message)
 *   }
 * }
 * ```
 */
export class DaytonaAuthorizationError extends DaytonaError {}

/**
 * Error thrown when a resource conflict occurs (HTTP 409).
 *
 * @example
 * ```ts
 * try {
 *   await daytona.create({ name: 'existing-sandbox' })
 * } catch (error) {
 *   if (error instanceof DaytonaConflictError) {
 *     console.log(error.code)
 *   }
 * }
 * ```
 */
export class DaytonaConflictError extends DaytonaError {}

/**
 * Error thrown when input validation fails (HTTP 400 or client-side validation).
 *
 * @example
 * ```ts
 * try {
 *   Image.debianSlim('3.8' as never)
 * } catch (error) {
 *   if (error instanceof DaytonaValidationError) {
 *     console.log(error.message)
 *   }
 * }
 * ```
 */
export class DaytonaValidationError extends DaytonaError {}

/**
 * Error thrown when a timeout occurs.
 *
 * @example
 * ```ts
 * try {
 *   await sandbox.waitUntilStarted(1)
 * } catch (error) {
 *   if (error instanceof DaytonaTimeoutError) {
 *     console.log(error.message)
 *   }
 * }
 * ```
 */
export class DaytonaTimeoutError extends DaytonaError {}

/**
 * Error thrown when a network connection fails.
 *
 * @example
 * ```ts
 * try {
 *   await ptyHandle.waitForConnection()
 * } catch (error) {
 *   if (error instanceof DaytonaConnectionError) {
 *     console.log(error.message)
 *   }
 * }
 * ```
 */
export class DaytonaConnectionError extends DaytonaError {}

/**
 * Map of (source, code) → DaytonaError subclass. The key is `source|code`,
 * which keeps the mapping unique across services even when two components
 * later use the same code string with different semantics.
 *
 * Entries with `*|code` (wildcard source) apply when the wire response omits
 * `source` or carries an unrecognized one.
 */
const CODE_TO_ERROR_CLASS: Record<string, typeof DaytonaError> = {
  'DAYTONA_DAEMON|GIT_AUTH_FAILED': DaytonaAuthenticationError,
  'DAYTONA_DAEMON|GIT_REPO_NOT_FOUND': DaytonaNotFoundError,
  'DAYTONA_DAEMON|GIT_BRANCH_NOT_FOUND': DaytonaNotFoundError,
  'DAYTONA_DAEMON|GIT_PUSH_REJECTED': DaytonaConflictError,
  'DAYTONA_DAEMON|GIT_BRANCH_EXISTS': DaytonaConflictError,
  'DAYTONA_DAEMON|GIT_DIRTY_WORKTREE': DaytonaConflictError,
  'DAYTONA_DAEMON|GIT_MERGE_CONFLICT': DaytonaConflictError,
  'DAYTONA_DAEMON|FILE_NOT_FOUND': DaytonaNotFoundError,
  'DAYTONA_DAEMON|FILE_ACCESS_DENIED': DaytonaAuthorizationError,
  'DAYTONA_DAEMON|LSP_SERVER_NOT_INITIALIZED': DaytonaValidationError,
  'DAYTONA_DAEMON|PROCESS_NOT_FOUND': DaytonaNotFoundError,
  'DAYTONA_DAEMON|PROCESS_EXECUTION_TIMEOUT': DaytonaTimeoutError,
  'DAYTONA_DAEMON|SESSION_ENDED': DaytonaNotFoundError,
  'DAYTONA_DAEMON|COMMAND_ALREADY_COMPLETED': DaytonaNotFoundError,
  'DAYTONA_DAEMON|A11Y_UNAVAILABLE': DaytonaConnectionError,
  'DAYTONA_DAEMON|RECORDING_STILL_ACTIVE': DaytonaConflictError,
  'DAYTONA_DAEMON|RECORDING_FFMPEG_NOT_FOUND': DaytonaConnectionError,
  // Proxy-originated errors. The proxy translates upstream errors at the
  // boundary, so these cover all preview-path failures the SDK sees.
  [`DAYTONA_PROXY|${ProxyErrorCode.SANDBOX_NOT_FOUND}`]: DaytonaNotFoundError,
  [`DAYTONA_PROXY|${ProxyErrorCode.SANDBOX_NOT_STARTED}`]: DaytonaValidationError,
  [`DAYTONA_PROXY|${ProxyErrorCode.RUNNER_UNREACHABLE}`]: DaytonaConnectionError,
}

function lookupErrorClass(source: string | undefined, code: string | undefined): typeof DaytonaError | undefined {
  if (!code) return undefined
  if (source) {
    const exact = CODE_TO_ERROR_CLASS[`${source}|${code}`]
    if (exact) return exact
  }
  return CODE_TO_ERROR_CLASS[`*|${code}`]
}

const STATUS_CODE_TO_ERROR: Record<number, typeof DaytonaError> = {
  400: DaytonaValidationError,
  401: DaytonaAuthenticationError,
  403: DaytonaAuthorizationError,
  404: DaytonaNotFoundError,
  409: DaytonaConflictError,
  429: DaytonaRateLimitError,
}

/**
 * Maps an HTTP status code to the corresponding Daytona error class.
 */
export function errorClassFromStatusCode(statusCode?: number): typeof DaytonaError {
  if (statusCode === undefined) {
    return DaytonaError
  }

  return STATUS_CODE_TO_ERROR[statusCode] || DaytonaError
}

/**
 * Creates the appropriate Daytona error subclass from structured error metadata.
 *
 * Resolution order:
 *   1. Precise (source, code) lookup.
 *   2. HTTP status code fallback.
 */
export function createDaytonaError(
  message: string,
  statusCode?: number,
  headers?: ResponseHeaders,
  code?: string,
  source?: string,
): DaytonaError {
  const ErrorClass = lookupErrorClass(source, code) || errorClassFromStatusCode(statusCode)
  return new ErrorClass(message, statusCode, headers, code, source)
}

function isAxiosTimeoutError(error: AxiosError): boolean {
  return error.code === 'ECONNABORTED' || error.code === 'ETIMEDOUT' || error.message.includes('timeout of')
}

function getAxiosResponseDataObject(error: AxiosError): Record<string, unknown> | undefined {
  if (!error.response?.data || typeof error.response.data !== 'object') {
    return undefined
  }

  return error.response.data as Record<string, unknown>
}

function extractAxiosErrorCode(responseData?: Record<string, unknown>): string | undefined {
  if (typeof responseData?.code === 'string') {
    return responseData.code
  }

  return undefined
}

function extractAxiosErrorSource(responseData?: Record<string, unknown>): string | undefined {
  return typeof responseData?.source === 'string' ? responseData.source : undefined
}

function extractAxiosErrorMessage(error: AxiosError): string {
  if (isAxiosTimeoutError(error)) {
    return 'Operation timed out'
  }

  const responseData = getAxiosResponseDataObject(error)
  const responseMessage: unknown = responseData?.message || error.response?.data
  const message: unknown = responseMessage || error.message || String(error)

  if (typeof message === 'object') {
    try {
      return JSON.stringify(message)
    } catch {
      return String(message)
    }
  }

  return String(message)
}

/**
 * Creates the appropriate Daytona error subclass from an Axios error.
 */
export function createAxiosDaytonaError(error: AxiosError): DaytonaError {
  const message = extractAxiosErrorMessage(error)
  const statusCode = error.response?.status
  const headers = error.response?.headers as ResponseHeaders | undefined
  const responseData = getAxiosResponseDataObject(error)
  const code = extractAxiosErrorCode(responseData)
  const source = extractAxiosErrorSource(responseData)

  if (isAxiosTimeoutError(error)) {
    return new DaytonaTimeoutError(message, statusCode, headers, code, source)
  }

  if (!error.response && (error.request || error.code)) {
    return new DaytonaConnectionError(message, statusCode, headers, code, source)
  }

  return createDaytonaError(message, statusCode, headers, code, source)
}
