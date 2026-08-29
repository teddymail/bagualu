package httptransport

import (
	"errors"
	"net/http"
	"strings"

	"github.com/teddymail/bagualu/internal/domain"
)

// writeNotFoundOrError writes 404 for domain.ErrNotFound, otherwise 500.
func writeNotFoundOrError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		apiErr(w, http.StatusNotFound, "not_found", "resource not found")
	} else {
		apiErr(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}

func isTestQueueFull(err error) bool {
	return err != nil && strings.Contains(err.Error(), "test_queue_full")
}

// nilSafeSlice returns an empty JSON-friendly slice when s is nil.
func nilSafeSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// domainNodeFilter is an alias type used so that callers can write a zero value
// without importing domain in files that don't already import it.
type domainNodeFilter = domain.NodeFilter

// domainJobFilter is an alias type for convenience.
type domainJobFilter = domain.JobFilter
