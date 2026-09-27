package receipt

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	apperrors "github.com/sonalshailja/receipt-autoitemize/internal/errors"
	"github.com/sonalshailja/receipt-autoitemize/internal/interceptors/requestid"
	"github.com/sonalshailja/receipt-autoitemize/internal/logger"
	"github.com/sonalshailja/receipt-autoitemize/pkg/id"
)

type uploadResponse struct {
	ReceiptID   string `json:"receipt_id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
}

// UploadHandler serves POST /receipts. grpc-gateway cannot bind multipart
// bodies, so this is a plain HTTP handler mounted on the gateway mux; it calls
// the same Service as the gRPC methods and renders errors in the same envelope.
func UploadHandler(service *Service, maxBytes int64) runtime.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		requestID := r.Header.Get(requestid.HeaderKey)
		if requestID == "" {
			requestID = id.New("req")
		}
		w.Header().Set(requestid.HeaderKey, requestID)
		ctx := requestid.WithContext(r.Context(), requestID)
		log := logger.FromContext(ctx).With("request_id", requestID, "method", "POST /receipts")

		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		file, header, err := r.FormFile("file")
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, requestID, apperrors.Newf(apperrors.KindInvalidArgument,
					"file_too_large", "the upload exceeds %d bytes", maxBytes))
				return
			}
			writeError(w, requestID, apperrors.New(apperrors.KindInvalidArgument,
				"file_required", `send the receipt as multipart form field "file"`))
			return
		}
		defer file.Close()

		rec, err := service.Upload(ctx, UploadInput{
			FileName:    header.Filename,
			ContentType: header.Header.Get("Content-Type"),
			Body:        file,
		})
		if err != nil {
			log.WarnContext(ctx, "upload rejected", "error", err.Error())
			writeError(w, requestID, err)
			return
		}

		log.InfoContext(ctx, "receipt uploaded", "receipt_id", rec.ID, "size_bytes", rec.SizeBytes)
		writeJSON(w, http.StatusCreated, uploadResponse{
			ReceiptID:   rec.ID,
			FileName:    rec.FileName,
			ContentType: rec.ContentType,
			SizeBytes:   rec.SizeBytes,
			SHA256:      rec.SHA256,
		})
	}
}

func writeError(w http.ResponseWriter, requestID string, err error) {
	status, body := apperrors.ResponseFor(err, requestID)
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
