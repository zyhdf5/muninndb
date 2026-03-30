"""MuninnDB error types."""


class MuninnError(Exception):
    """Base exception for all MuninnDB errors."""

    def __init__(self, message: str, status_code: int | None = None):
        super().__init__(message)
        self.status_code = status_code


class MuninnConnectionError(MuninnError):
    """Connection-related errors (network, SSL, DNS)."""

    pass


class MuninnAuthError(MuninnError):
    """Authentication failed (401 Unauthorized)."""

    pass


class MuninnNotFound(MuninnError):
    """Resource not found (404 Not Found)."""

    pass


class MuninnConflict(MuninnError):
    """Request conflict (409 Conflict)."""

    pass


class MuninnServerError(MuninnError):
    """Server error (5xx)."""

    pass


class MuninnTimeoutError(MuninnError):
    """Request timeout."""

    pass
