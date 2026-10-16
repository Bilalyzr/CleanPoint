// Package api exposes the authenticated REST surface of CleanPoint under
// /api/v1 using only the Go standard library router.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"cleanpoint/internal/domain"
	"cleanpoint/internal/restore"
	"cleanpoint/internal/security"
	"cleanpoint/internal/snapshots"
	"cleanpoint/internal/storage"
)

// APIError is the uniform error convention: {code, message, details,
// request_id}. Messages stay operator-useful but never leak host-absolute
// paths.
type APIError struct {
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	Details   []string `json:"details,omitempty"`
	RequestID string   `json:"request_id"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string, details ...string) {
	rid := requestID(r)
	if len(details) == 0 {
		details = nil
	}
	writeJSON(w, status, APIError{Code: code, Message: msg, Details: details, RequestID: rid})
}

// mapError converts internal errors into the API error convention.
func mapError(w http.ResponseWriter, r *http.Request, err error) {
	var details []string
	msg := err.Error()
	switch {
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", "requested resource does not exist")
	case errors.Is(err, storage.ErrConflict):
		writeError(w, r, http.StatusConflict, "conflict", msg)
	case errors.Is(err, restore.ErrIneligible):
		writeError(w, r, http.StatusConflict, "not_eligible", msg)
	case errors.Is(err, errBusy):
		writeError(w, r, http.StatusConflict, "busy", "another capture/restore operation is in progress; retry shortly")
	case errors.Is(err, restore.ErrConfirmRequired):
		writeError(w, r, http.StatusBadRequest, "confirm_required", msg)
	case errors.Is(err, snapshots.ErrTooManyFiles), errors.Is(err, snapshots.ErrTooManyBytes):
		writeError(w, r, http.StatusRequestEntityTooLarge, "limit_exceeded", msg)
	case errors.Is(err, snapshots.ErrRootUnavailable):
		writeError(w, r, http.StatusUnprocessableEntity, "root_unavailable", msg)
	case errors.Is(err, snapshots.ErrNoEvents):
		writeError(w, r, http.StatusBadRequest, "no_telemetry", msg)
	case isSecurityPathError(err):
		details = append(details, "path rejected by sandbox policy")
		writeError(w, r, http.StatusUnprocessableEntity, "unsafe_path", msg, details...)
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, r, http.StatusBadRequest, "invalid_input", msg)
	default:
		log.Printf("ERROR %s %s internal: %v (request %s)", r.Method, r.URL.Path, err, requestID(r))
		writeError(w, r, http.StatusInternalServerError, "internal", "internal error; see server logs")
	}
}

func isSecurityPathError(err error) bool {
	return errors.Is(err, security.ErrEmptyPath) || errors.Is(err, security.ErrAbsolutePath) ||
		errors.Is(err, security.ErrTraversal) || errors.Is(err, security.ErrDevicePath) ||
		errors.Is(err, security.ErrNonRegular) || errors.Is(err, security.ErrSymlink) ||
		errors.Is(err, security.ErrDotComponent) || errors.Is(err, security.ErrNullByte) ||
		errors.Is(err, security.ErrOutsideSandbox)
}

func badRequest(w http.ResponseWriter, r *http.Request, format string, args ...any) {
	writeError(w, r, http.StatusBadRequest, "invalid_input", fmt.Sprintf(format, args...))
}
