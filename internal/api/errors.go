package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// ErrUnavailable is a feature that is not configured.
var ErrUnavailable = errors.New("unavailable")

var errNotFoundRoute = &requestError{code: wire.CodeNotFound, status: http.StatusNotFound, msg: "no such endpoint"}

// requestError is a failure the API itself detects in a request.
type requestError struct {
	code   string
	status int
	msg    string
	field  string
}

func (e *requestError) Error() string { return e.msg }

func badRequest(field, format string, args ...any) error {
	return &requestError{code: wire.CodeInvalidRequest, status: http.StatusBadRequest,
		msg: fmt.Sprintf(format, args...), field: field}
}

// stateError is a request the engine's state refuses outside the engine: an
// edit of the open segment, or deleting what the open segment is tracking.
type stateError struct{ msg string }

func (e *stateError) Error() string        { return e.msg }
func (e *stateError) Is(target error) bool { return target == timeengine.ErrInvalidState }

// fail writes the error response for err. It is the only place errors become
// HTTP status codes (docs/04-api-contract.md#errors).
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	body := wire.ErrorBody{Details: map[string]any{}}
	status := http.StatusInternalServerError
	var (
		re *requestError
		ke *config.KeyError
		se *store.Error
	)
	switch {
	case errors.As(err, &re):
		status, body.Code, body.Message = re.status, re.code, re.msg
		if re.field != "" {
			body.Details["field"] = re.field
		}
	case errors.As(err, &ke):
		status, body.Code, body.Message = http.StatusBadRequest, wire.CodeInvalidRequest, ke.Error()
		body.Details["key"] = ke.Key
	case errors.Is(err, timeengine.ErrInvalidState):
		status, body.Code, body.Message = http.StatusUnprocessableEntity, wire.CodeInvalidState, userMessage(err)
		if snap, serr := s.Tracker.Snapshot(r.Context()); serr == nil {
			body.Details["state"] = string(snap.State)
		}
	case errors.Is(err, store.ErrNotFound):
		status, body.Code, body.Message = http.StatusNotFound, wire.CodeNotFound, "not found"
	case errors.Is(err, store.ErrConflict):
		status, body.Code, body.Message = http.StatusConflict, wire.CodeConflict, "that conflicts with other data"
	case errors.Is(err, store.ErrInvalid):
		status, body.Code, body.Message = http.StatusBadRequest, wire.CodeInvalidRequest, "invalid value"
	case errors.Is(err, ErrUnavailable):
		status, body.Code, body.Message = http.StatusServiceUnavailable, wire.CodeUnavailable, userMessage(err)
	default:
		body.Code, body.Message = wire.CodeInternal, "internal error"
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	if errors.As(err, &se) && body.Code != wire.CodeInternal {
		body.Message = se.Message
		if se.Field != "" {
			body.Details["field"] = se.Field
		}
	}
	writeJSON(w, status, wire.ErrorResponse{Error: body})
}

// userMessage is the message of an error meant for the user: the engine's
// state errors and the API's own are written to be shown as they are.
func userMessage(err error) string {
	var st *stateError
	if errors.As(err, &st) {
		return st.msg
	}
	var un *unavailableError
	if errors.As(err, &un) {
		return un.msg
	}
	return err.Error()
}

// unavailableError is a feature that is not configured, with a message naming
// how to set it up.
type unavailableError struct{ msg string }

func (e *unavailableError) Error() string        { return e.msg }
func (e *unavailableError) Is(target error) bool { return target == ErrUnavailable }

// Unavailable returns an error answered with 503 unavailable and msg.
func Unavailable(format string, args ...any) error {
	return &unavailableError{msg: fmt.Sprintf(format, args...)}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("write response", "err", err)
	}
}

// decode reads a JSON body strictly: an unknown field, a wrong type, or
// trailing data is invalid_request naming the field where possible.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var te *json.UnmarshalTypeError
		switch {
		case errors.Is(err, io.EOF):
			return badRequest("", "the request needs a JSON body; send {} for none")
		case errors.As(err, &te):
			return badRequest(te.Field, "%s has the wrong type", te.Field)
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
			return badRequest(field, "unknown field %s", field)
		}
		return badRequest("", "malformed JSON: %v", err)
	}
	if dec.More() {
		return badRequest("", "malformed JSON: trailing data")
	}
	return nil
}
