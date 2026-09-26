package wire

// Error codes (docs/04-api-contract.md#errors).
const (
	CodeInvalidRequest = "invalid_request"
	CodeNotFound       = "not_found"
	CodeConflict       = "conflict"
	CodeInvalidState   = "invalid_state"
	CodeUnavailable    = "unavailable"
	CodeInternal       = "internal"
)

// ErrorResponse is the body of every non-2xx response.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody describes the failure. Message is a lowercase sentence safe to show
// to the user. Details is always an object, empty when there is nothing to add;
// details.field names the offending request field, details.key the offending
// config key, and details.state the engine state.
type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}
