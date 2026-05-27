# Copyright 2025 Daytona Platforms Inc.
# SPDX-License-Identifier: Apache-2.0
from __future__ import annotations

import functools
import inspect
import json
from collections.abc import AsyncIterator, Awaitable, Callable, Iterator, Mapping
from typing import Any, NoReturn, TypeVar, Union, cast

import httpx

from daytona_api_client.exceptions import (
    BadRequestException,
    ConflictException,
    ForbiddenException,
    NotFoundException,
    OpenApiException,
    UnauthorizedException,
)
from daytona_api_client_async.exceptions import BadRequestException as BadRequestExceptionAsync
from daytona_api_client_async.exceptions import ConflictException as ConflictExceptionAsync
from daytona_api_client_async.exceptions import ForbiddenException as ForbiddenExceptionAsync
from daytona_api_client_async.exceptions import NotFoundException as NotFoundExceptionAsync
from daytona_api_client_async.exceptions import OpenApiException as OpenApiExceptionAsync
from daytona_api_client_async.exceptions import UnauthorizedException as UnauthorizedExceptionAsync
from daytona_toolbox_api_client.exceptions import BadRequestException as BadRequestExceptionToolbox
from daytona_toolbox_api_client.exceptions import ConflictException as ConflictExceptionToolbox
from daytona_toolbox_api_client.exceptions import ForbiddenException as ForbiddenExceptionToolbox
from daytona_toolbox_api_client.exceptions import NotFoundException as NotFoundExceptionToolbox
from daytona_toolbox_api_client.exceptions import OpenApiException as OpenApiExceptionToolbox
from daytona_toolbox_api_client.exceptions import UnauthorizedException as UnauthorizedExceptionToolbox
from daytona_toolbox_api_client_async.exceptions import BadRequestException as BadRequestExceptionToolboxAsync
from daytona_toolbox_api_client_async.exceptions import ConflictException as ConflictExceptionToolboxAsync
from daytona_toolbox_api_client_async.exceptions import ForbiddenException as ForbiddenExceptionToolboxAsync
from daytona_toolbox_api_client_async.exceptions import NotFoundException as NotFoundExceptionToolboxAsync
from daytona_toolbox_api_client_async.exceptions import OpenApiException as OpenApiExceptionToolboxAsync
from daytona_toolbox_api_client_async.exceptions import UnauthorizedException as UnauthorizedExceptionToolboxAsync

from daytona_toolbox_api_client.models.daemon_error_code import DaemonErrorCode

from .._generated.proxy_error_code import ProxyErrorCode
from ..common.errors import (
    DaytonaAuthenticationError,
    DaytonaAuthorizationError,
    DaytonaConflictError,
    DaytonaConnectionError,
    DaytonaError,
    DaytonaNotFoundError,
    DaytonaTimeoutError,
    DaytonaValidationError,
)
from ..common.errors import create_daytona_error as create_daytona_error_from_status_code
from ..common.errors import error_class_from_status_code
from .types import has_body

# Map (source, code) tuples to the precise DaytonaError subclass to raise.
# A None source means "match any source" — useful for codes that are
# unambiguous regardless of where they originate (today the catalog has none,
# but the structure is future-proof against cross-component collisions like
# `SANDBOX_NOT_FOUND` appearing in both proxy and runner).
_CODE_TO_EXCEPTION: dict[tuple[str | None, str], type[DaytonaError]] = {
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeGitAuthFailed.value): DaytonaAuthenticationError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeGitRepoNotFound.value): DaytonaNotFoundError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeGitBranchNotFound.value): DaytonaNotFoundError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeGitPushRejected.value): DaytonaConflictError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeGitBranchExists.value): DaytonaConflictError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeGitDirtyWorktree.value): DaytonaConflictError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeGitMergeConflict.value): DaytonaConflictError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeFileNotFound.value): DaytonaNotFoundError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeFileAccessDenied.value): DaytonaAuthorizationError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeLspServerNotInitialized.value): DaytonaValidationError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeProcessExecutionTimeout.value): DaytonaTimeoutError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeProcessNotFound.value): DaytonaNotFoundError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeSessionEnded.value): DaytonaNotFoundError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeCommandAlreadyCompleted.value): DaytonaNotFoundError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeA11yUnavailable.value): DaytonaConnectionError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeRecordingStillActive.value): DaytonaConflictError,
    ("DAYTONA_DAEMON", DaemonErrorCode.CodeRecordingFfmpegNotFound.value): DaytonaConnectionError,
    # Proxy-originated errors. The proxy translates upstream errors at the
    # boundary, so these cover all preview-path failures the SDK sees.
    ("DAYTONA_PROXY", ProxyErrorCode.SANDBOX_NOT_FOUND.value): DaytonaNotFoundError,
    ("DAYTONA_PROXY", ProxyErrorCode.SANDBOX_NOT_STARTED.value): DaytonaValidationError,
    ("DAYTONA_PROXY", ProxyErrorCode.RUNNER_UNREACHABLE.value): DaytonaConnectionError,
}


def _lookup_exception_class(source: str | None, code: str | None) -> type[DaytonaError] | None:
    """Resolve a typed exception class from the (source, code) wire identifier.

    Falls back to ``(None, code)`` for source-agnostic mappings so a code that
    later moves between services keeps the same exception type.
    """
    if not code:
        return None
    if source is not None:
        cls = _CODE_TO_EXCEPTION.get((source, code))
        if cls is not None:
            return cls
    return _CODE_TO_EXCEPTION.get((None, code))

SESSION_IS_CLOSED_ERROR_MESSAGE = "Session is closed"

F = TypeVar("F", bound=Callable[..., object])
OpenApiDaytonaException = Union[
    OpenApiException,
    OpenApiExceptionAsync,
    OpenApiExceptionToolbox,
    OpenApiExceptionToolboxAsync,
]

OPENAPI_EXCEPTIONS = (OpenApiException, OpenApiExceptionAsync, OpenApiExceptionToolbox, OpenApiExceptionToolboxAsync)
NOT_FOUND_EXCEPTIONS = (
    NotFoundException,
    NotFoundExceptionAsync,
    NotFoundExceptionToolbox,
    NotFoundExceptionToolboxAsync,
)
UNAUTHORIZED_EXCEPTIONS = (
    UnauthorizedException,
    UnauthorizedExceptionAsync,
    UnauthorizedExceptionToolbox,
    UnauthorizedExceptionToolboxAsync,
)
FORBIDDEN_EXCEPTIONS = (
    ForbiddenException,
    ForbiddenExceptionAsync,
    ForbiddenExceptionToolbox,
    ForbiddenExceptionToolboxAsync,
)
BAD_REQUEST_EXCEPTIONS = (
    BadRequestException,
    BadRequestExceptionAsync,
    BadRequestExceptionToolbox,
    BadRequestExceptionToolboxAsync,
)
CONFLICT_EXCEPTIONS = (
    ConflictException,
    ConflictExceptionAsync,
    ConflictExceptionToolbox,
    ConflictExceptionToolboxAsync,
)
TRANSPORT_ERROR_TO_DAYTONA_ERROR: tuple[tuple[type[BaseException], type[DaytonaError]], ...] = (
    (httpx.TimeoutException, DaytonaTimeoutError),
    (httpx.NetworkError, DaytonaConnectionError),
    (TimeoutError, DaytonaTimeoutError),
    # ConnectionError covers ConnectionRefusedError, ConnectionResetError, etc.
    # It intentionally does not catch the broader OSError family.
    (ConnectionError, DaytonaConnectionError),
)


def _prefix_message(message_prefix: str, message: str) -> str:
    """Apply an optional prefix to an error message."""

    if not message_prefix:
        return message

    return f"{message_prefix}{message}"


def intercept_errors(
    message_prefix: str = "",
) -> Callable[[F], F]:
    """Decorator to intercept errors, process them, and optionally add a message prefix.
    If the error is an OpenApiException, it will be processed to extract the most meaningful error message.

    Args:
        message_prefix (str): Custom message prefix for the error.
    """

    def decorator(func: F) -> F:
        def process_n_raise_exception(e: Exception) -> NoReturn:
            if isinstance(e, DaytonaError):
                raise e.__class__(
                    _prefix_message(message_prefix, str(e)),
                    status_code=e.status_code,
                    headers=e.headers,
                    code=e.code,
                    source=e.source,
                ) from None

            if isinstance(e, OPENAPI_EXCEPTIONS):
                msg, code, source = _get_open_api_exception_message(e)
                status_code = getattr(e, "status", None)
                headers = cast(Mapping[str, Any] | None, getattr(e, "headers", None))

                raise create_daytona_error(
                    _prefix_message(message_prefix, msg),
                    status_code=status_code,
                    headers=headers,
                    code=code,
                    source=source,
                    exception=e,
                ) from None

            for source_error, daytona_error_cls in TRANSPORT_ERROR_TO_DAYTONA_ERROR:
                if isinstance(e, source_error):
                    raise daytona_error_cls(_prefix_message(message_prefix, str(e))) from None

            if isinstance(e, RuntimeError) and SESSION_IS_CLOSED_ERROR_MESSAGE in str(e):
                raise DaytonaError(
                    (
                        f"{_prefix_message(message_prefix, str(e))}: Daytona client is closed"
                        " — sandbox is used outside its parent's context. "
                        "Ensure sandboxes are only used within the scope of their parent Daytona object."
                    )
                ) from e

            raise DaytonaError(_prefix_message(message_prefix, str(e)))  # pylint: disable=raise-missing-from

        if inspect.isasyncgenfunction(func):
            async_gen_func = cast(Callable[..., AsyncIterator[Any]], func)

            @functools.wraps(func)
            async def async_gen_wrapper(*args: object, **kwargs: object) -> AsyncIterator[Any]:
                try:
                    async for item in async_gen_func(*args, **kwargs):
                        yield item
                except Exception as e:
                    process_n_raise_exception(e)

            return cast(F, async_gen_wrapper)

        if inspect.isgeneratorfunction(func):
            sync_gen_func = cast(Callable[..., Iterator[Any]], func)

            @functools.wraps(func)
            def sync_gen_wrapper(*args: object, **kwargs: object) -> Iterator[Any]:
                try:
                    yield from sync_gen_func(*args, **kwargs)
                except Exception as e:
                    process_n_raise_exception(e)

            return cast(F, sync_gen_wrapper)

        if inspect.iscoroutinefunction(func):
            async_func = cast(Callable[..., Awaitable[object]], func)

            @functools.wraps(func)
            async def async_wrapper(*args: object, **kwargs: object) -> object:
                try:
                    return await async_func(*args, **kwargs)
                except Exception as e:
                    process_n_raise_exception(e)

            return cast(F, async_wrapper)

        sync_func = cast(Callable[..., object], func)

        @functools.wraps(func)
        def sync_wrapper(*args: object, **kwargs: object) -> object:
            try:
                return sync_func(*args, **kwargs)
            except Exception as e:
                process_n_raise_exception(e)

        return cast(F, sync_wrapper)

    return decorator


def _map_api_exception_to_error(
    e: OpenApiDaytonaException,
    status_code: int | None,
    code: str | None = None,
    source: str | None = None,
) -> type[DaytonaError]:
    """Map an OpenAPI exception to the appropriate DaytonaError subclass.

    Resolution order:
    1. Precise (source, code) lookup — surfaces the most specific cross-SDK
       exception class available for the wire error identifier.
    2. OpenAPI exception type — preserves the original HTTP-status classification
       even when the generated client uses a domain-specific subclass.
    3. HTTP status code fallback.
    """
    typed = _lookup_exception_class(source, code)
    if typed is not None:
        return typed

    if isinstance(e, NOT_FOUND_EXCEPTIONS):
        return DaytonaNotFoundError

    if isinstance(e, UNAUTHORIZED_EXCEPTIONS):
        return DaytonaAuthenticationError

    if isinstance(e, FORBIDDEN_EXCEPTIONS):
        return DaytonaAuthorizationError

    if isinstance(e, BAD_REQUEST_EXCEPTIONS):
        return DaytonaValidationError

    if isinstance(e, CONFLICT_EXCEPTIONS):
        return DaytonaConflictError

    return error_class_from_status_code(status_code)


def create_daytona_error(
    message: str,
    status_code: int | None = None,
    headers: Mapping[str, Any] | None = None,
    code: str | None = None,
    source: str | None = None,
    exception: OpenApiDaytonaException | None = None,
) -> DaytonaError:
    """Create the appropriate DaytonaError subclass from structured error metadata."""

    if exception is None:
        return create_daytona_error_from_status_code(
            message,
            status_code=status_code,
            headers=headers,
            code=code,
            source=source,
        )

    error_cls = _map_api_exception_to_error(exception, status_code, code=code, source=source)
    return error_cls(
        message,
        status_code=status_code,
        headers=headers,
        code=code,
        source=source,
    )


def _get_open_api_exception_message(
    exception: OpenApiDaytonaException,
) -> tuple[str, str | None, str | None]:
    """Process API exceptions to extract the most meaningful error metadata.

    This method examines the exception's body attribute and attempts to extract
    the most informative error message using the following logic:
    1. If the body is missing or empty, returns the original exception
    2. If the body contains valid JSON with a 'message' field, uses that message
    3. If the body is not valid JSON or does not contain a 'message' field, uses the raw body string

    Args:
        exception (OpenApiException): The OpenApiException to process

    Returns:
        Tuple of (message, code, source).
    """
    if not has_body(exception):
        return str(exception), None, None

    body_str: str = str(exception.body)
    message: str = body_str
    code: str | None = None
    source: str | None = None
    try:
        data = json.loads(body_str)
        if isinstance(data, dict):
            typed_data: dict[str, object] = cast(dict[str, object], data)
            msg: object | None = typed_data.get("message")
            if isinstance(msg, str):
                message = msg
            code_value: object | None = typed_data.get("code")
            if not isinstance(code_value, str):
                code_value = typed_data.get("error_code")
            if isinstance(code_value, str):
                code = code_value
            source_value: object | None = typed_data.get("source")
            if isinstance(source_value, str):
                source = source_value
    except json.JSONDecodeError:
        pass

    return message, code, source
