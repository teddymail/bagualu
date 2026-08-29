package mihomo

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/teddymail/bagualu/internal/domain"
)

// KernelError wraps a stable domain error code with an HTTP status code and cause.
type KernelError struct {
	Code       string
	HTTPStatus int // 0 if not an HTTP-level error
	Cause      error
}

func (e *KernelError) ErrorCode() string { return e.Code }

func (e *KernelError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return e.Code
}

func (e *KernelError) Unwrap() error { return e.Cause }

// ErrorCode extracts the stable error code from an error (or returns "").
func ErrorCode(err error) string {
	var ke *KernelError
	if errors.As(err, &ke) {
		return ke.Code
	}
	return ""
}

func kernelErr(code string, cause error) *KernelError {
	return &KernelError{Code: code, Cause: cause}
}

// mapHTTPStatus maps an HTTP status code to a stable domain error code.
// 404 on proxy endpoints → node_load_failed; auth/JSON/other → core_api_unavailable.
func mapHTTPStatus(status int) string {
	switch {
	case status == http.StatusNotFound:
		return domain.ErrCodeNodeLoadFailed
	case status == http.StatusUnauthorized:
		return domain.ErrCodeCoreAPIUnavailable
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return domain.ErrCodeNodeLoadFailed
	default:
		return domain.ErrCodeCoreAPIUnavailable
	}
}
