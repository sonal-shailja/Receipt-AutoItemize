package boot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	apperrors "github.com/sonalshailja/receipt-autoitemize/internal/errors"
	"github.com/sonalshailja/receipt-autoitemize/internal/interceptors/requestid"
)

func TestGatewayErrorHandler_ConflictCarriesDetails(t *testing.T) {
	err := apperrors.ToGRPCStatus(apperrors.New(apperrors.KindConflict, "items_total_mismatch", "does not add up").
		WithDetails(map[string]string{"difference": "0.85"}))
	ctx := requestid.WithContext(context.Background(), "req_1")
	rec := httptest.NewRecorder()

	gatewayErrorHandler(ctx, nil, &runtime.JSONPb{}, rec, httptest.NewRequest(http.MethodPatch, "/", nil), err)

	assert.Equal(t, http.StatusConflict, rec.Code)
	var body apperrors.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, apperrors.ResponseBody{
		Code: "items_total_mismatch", Message: "does not add up", RequestID: "req_1",
		Details: map[string]string{"difference": "0.85"},
	}, body.Error)
}

func TestGatewayErrorHandler_Unprocessable(t *testing.T) {
	err := apperrors.ToGRPCStatus(apperrors.New(apperrors.KindUnprocessable, "ocr_failed", "could not read"))
	rec := httptest.NewRecorder()

	gatewayErrorHandler(context.Background(), nil, &runtime.JSONPb{}, rec, httptest.NewRequest(http.MethodPost, "/", nil), err)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestGatewayErrorHandler_RequestIDFromGRPCHeader(t *testing.T) {
	// The gateway calls the handler with the HTTP request context; the request
	// id only exists in the header metadata the gRPC interceptor sent back.
	err := apperrors.ToGRPCStatus(apperrors.New(apperrors.KindNotFound, "transaction_not_found", "missing"))
	ctx := runtime.NewServerMetadataContext(context.Background(), runtime.ServerMetadata{
		HeaderMD: metadata.Pairs(requestid.HeaderKey, "req_9"),
	})
	rec := httptest.NewRecorder()

	gatewayErrorHandler(ctx, nil, &runtime.JSONPb{}, rec, httptest.NewRequest(http.MethodGet, "/", nil), err)

	var body apperrors.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "req_9", body.Error.RequestID)
}
