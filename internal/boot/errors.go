package boot

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/status"

	apperrors "github.com/sonalshailja/receipt-autoitemize/internal/errors"
	"github.com/sonalshailja/receipt-autoitemize/internal/interceptors/requestid"
)

// gatewayErrorHandler renders REST errors in the apperrors.Response envelope,
// with the status from apperrors.HTTPStatusForCode and any structured details
// (such as a reconciliation mismatch) carried through from the gRPC status.
func gatewayErrorHandler(
	ctx context.Context,
	_ *runtime.ServeMux,
	marshaler runtime.Marshaler,
	w http.ResponseWriter,
	_ *http.Request,
	err error,
) {
	st := status.Convert(err)

	code := apperrors.CodeFromStatus(err)
	if code == "" {
		// No structured detail: an internal error, or one raised by the gateway
		// itself before reaching a handler, such as malformed JSON.
		code = "internal_error"
		if st.Code() != 0 {
			code = st.Code().String()
		}
	}

	body := apperrors.Response{Error: apperrors.ResponseBody{
		Code:      code,
		Message:   st.Message(),
		RequestID: requestid.FromContext(ctx),
		Details:   apperrors.DetailsFromStatus(err),
	}}

	w.Header().Set("Content-Type", marshaler.ContentType(body))
	w.WriteHeader(apperrors.HTTPStatusForCode(st.Code()))
	_ = json.NewEncoder(w).Encode(body)
}
