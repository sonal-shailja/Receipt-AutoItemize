// Package errors defines the service's error taxonomy. Business code raises
// errors by Kind; the transport edge maps a Kind to an HTTP or gRPC status.
package errors

import (
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Kind classifies a failure. It is the only thing the transport layer inspects.
type Kind string

const (
	// KindInvalidArgument is a malformed or semantically invalid request.
	KindInvalidArgument Kind = "invalid_argument"
	// KindNotFound is a lookup that matched nothing.
	KindNotFound Kind = "not_found"
	// KindConflict is a request that collided with the current state.
	KindConflict Kind = "conflict"
	// KindUnprocessable is a well-formed request whose input could not be
	// processed, such as a receipt OCR could not read.
	KindUnprocessable Kind = "unprocessable"
	// KindExhausted is a valid request that no resource can currently satisfy.
	KindExhausted Kind = "exhausted"
	// KindUnavailable is a transient dependency failure worth retrying.
	KindUnavailable Kind = "unavailable"
	// KindInternal is a bug or an unclassified failure.
	KindInternal Kind = "internal"
)

// Error is a domain error: a Kind, a stable machine-readable code, a message
// safe to return to a caller, and an optional wrapped cause.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	// Details carries structured context for the caller, e.g. the amounts
	// behind a reconciliation conflict.
	Details map[string]string
	cause   error
}

// New builds an Error.
func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Newf builds an Error with a formatted message.
func Newf(kind Kind, code, format string, args ...any) *Error {
	return &Error{Kind: kind, Code: code, Message: fmt.Sprintf(format, args...)}
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap exposes the cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.cause }

// Wrap attaches an underlying cause and returns the same error, so call sites
// read as a single expression.
func (e *Error) Wrap(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

// WithMessage overrides the caller-facing message.
func (e *Error) WithMessage(message string) *Error {
	clone := *e
	clone.Message = message
	return &clone
}

// WithDetails attaches caller-facing structured context.
func (e *Error) WithDetails(details map[string]string) *Error {
	clone := *e
	clone.Details = details
	return &clone
}

// Is treats two Errors as equal when their codes match.
func (e *Error) Is(target error) bool {
	var other *Error
	if !errors.As(target, &other) {
		return false
	}
	return e.Code == other.Code
}

// KindOf reports the Kind of err, defaulting to KindInternal for errors that did
// not originate in this package.
func KindOf(err error) Kind {
	var domain *Error
	if errors.As(err, &domain) {
		return domain.Kind
	}
	return KindInternal
}

// HTTPStatus maps err onto an HTTP status code.
func HTTPStatus(err error) int {
	return HTTPStatusForCode(GRPCCode(err))
}

// HTTPStatusForCode maps a gRPC code onto an HTTP status code. It is separate
// from HTTPStatus because at the REST edge the error is already a gRPC status
// and the Kind is no longer available.
//
// ResourceExhausted maps to 409 rather than grpc-gateway's default of 429, and
// FailedPrecondition (KindUnprocessable) to 422 rather than 400.
func HTTPStatusForCode(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists, codes.Aborted, codes.ResourceExhausted:
		return http.StatusConflict
	case codes.FailedPrecondition:
		return http.StatusUnprocessableEntity
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.Unimplemented:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

// GRPCCode maps err onto a gRPC status code.
func GRPCCode(err error) codes.Code {
	switch KindOf(err) {
	case KindInvalidArgument:
		return codes.InvalidArgument
	case KindNotFound:
		return codes.NotFound
	case KindConflict:
		return codes.AlreadyExists
	case KindExhausted:
		return codes.ResourceExhausted
	case KindUnprocessable:
		return codes.FailedPrecondition
	case KindUnavailable:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}

// ErrorDomain namespaces the ErrorInfo details this service attaches.
const ErrorDomain = "receipt-autoitemize"

// ToGRPCStatus converts err into a gRPC status error, carrying the machine
// readable code as an ErrorInfo detail. Internal errors are flattened to a
// generic message so no internal detail reaches the caller.
func ToGRPCStatus(err error) error {
	if err == nil {
		return nil
	}

	var domain *Error
	if !errors.As(err, &domain) || domain.Kind == KindInternal {
		return status.Error(codes.Internal, "internal error")
	}

	st := status.New(GRPCCode(err), domain.Message)

	withDetails, detailErr := st.WithDetails(&errdetails.ErrorInfo{
		Reason:   domain.Code,
		Domain:   ErrorDomain,
		Metadata: domain.Details,
	})
	if detailErr != nil {
		return st.Err()
	}

	return withDetails.Err()
}

// CodeFromStatus extracts the machine-readable code from a gRPC status error,
// returning "" when there is none.
func CodeFromStatus(err error) string {
	st, ok := status.FromError(err)
	if !ok {
		return ""
	}

	for _, detail := range st.Details() {
		if info, isErrorInfo := detail.(*errdetails.ErrorInfo); isErrorInfo {
			return info.GetReason()
		}
	}

	return ""
}

// DetailsFromStatus extracts the structured details from a gRPC status error.
func DetailsFromStatus(err error) map[string]string {
	st, ok := status.FromError(err)
	if !ok {
		return nil
	}
	for _, detail := range st.Details() {
		if info, isErrorInfo := detail.(*errdetails.ErrorInfo); isErrorInfo && len(info.GetMetadata()) > 0 {
			return info.GetMetadata()
		}
	}
	return nil
}

// Response is the JSON body every REST failure returns. Code is stable and
// safe to branch on; Message is prose and may change.
type Response struct {
	Error ResponseBody `json:"error"`
}

// ResponseBody is the inside of Response.
type ResponseBody struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

// ResponseFor renders err for a handler that is not behind the gateway.
// Internal errors are flattened exactly as ToGRPCStatus does.
func ResponseFor(err error, requestID string) (int, Response) {
	body := ResponseBody{Code: "internal_error", Message: "internal error", RequestID: requestID}

	var domain *Error
	if errors.As(err, &domain) && domain.Kind != KindInternal {
		body.Code, body.Message, body.Details = domain.Code, domain.Message, domain.Details
	}
	return HTTPStatus(err), Response{Error: body}
}
