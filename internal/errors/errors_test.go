package errors

import (
	stderrors "errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToGRPCStatus_CarriesCodeAndDetails(t *testing.T) {
	err := New(KindConflict, "items_total_mismatch", "does not add up").
		WithDetails(map[string]string{"difference": "0.85"})

	st := ToGRPCStatus(err)

	assert.Equal(t, codes.AlreadyExists, status.Code(st))
	assert.Equal(t, "items_total_mismatch", CodeFromStatus(st))
	assert.Equal(t, map[string]string{"difference": "0.85"}, DetailsFromStatus(st))
	assert.Equal(t, http.StatusConflict, HTTPStatusForCode(status.Code(st)))
}

func TestToGRPCStatus_FlattensInternal(t *testing.T) {
	st := ToGRPCStatus(New(KindInternal, "db_failed", "select * from secrets").Wrap(stderrors.New("boom")))

	assert.Equal(t, codes.Internal, status.Code(st))
	assert.Equal(t, "internal error", status.Convert(st).Message())
	assert.Empty(t, CodeFromStatus(st))
}

func TestHTTPStatus(t *testing.T) {
	cases := map[error]int{
		New(KindInvalidArgument, "x", "x"): http.StatusBadRequest,
		New(KindNotFound, "x", "x"):        http.StatusNotFound,
		New(KindConflict, "x", "x"):        http.StatusConflict,
		New(KindUnprocessable, "x", "x"):   http.StatusUnprocessableEntity,
		New(KindInternal, "x", "x"):        http.StatusInternalServerError,
		stderrors.New("plain"):             http.StatusInternalServerError,
	}
	for err, want := range cases {
		assert.Equal(t, want, HTTPStatus(err), err.Error())
	}
}

func TestResponseFor(t *testing.T) {
	code, body := ResponseFor(New(KindInvalidArgument, "file_required", "missing file"), "req_1")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, ResponseBody{Code: "file_required", Message: "missing file", RequestID: "req_1"}, body.Error)

	code, body = ResponseFor(stderrors.New("disk on fire"), "req_2")
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.Equal(t, "internal_error", body.Error.Code)
	assert.Equal(t, "internal error", body.Error.Message)
}
