package receipt

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/sonalshailja/receipt-autoitemize/internal/errors"
)

func multipartRequest(t *testing.T, field, name, contentType string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+name+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/receipts", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestUploadHandler_Created(t *testing.T) {
	h := newHarness(t)
	content, err := os.ReadFile(fixturesDir + "/receipt-clean.txt")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	UploadHandler(h.svc, 1<<20)(rec, multipartRequest(t, "file", "receipt-clean.txt", "text/plain", content), nil)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var got uploadResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Contains(t, got.ReceiptID, "rcpt_")
	assert.Equal(t, "receipt-clean.txt", got.FileName)
	assert.Equal(t, int64(len(content)), got.SizeBytes)
	assert.NotEmpty(t, rec.Header().Get("x-request-id"))
}

func TestUploadHandler_Errors(t *testing.T) {
	h := newHarness(t)

	cases := []struct {
		name   string
		req    *http.Request
		max    int64
		status int
		code   string
	}{
		{"missing file field", multipartRequest(t, "document", "a.txt", "text/plain", []byte("x")), 1 << 20, 400, "file_required"},
		{"too large", multipartRequest(t, "file", "a.txt", "text/plain", bytes.Repeat([]byte("x"), 4096)), 1024, 400, "file_too_large"},
		{"unsupported type", multipartRequest(t, "file", "a.exe", "application/x-msdownload", []byte("MZ")), 1 << 20, 400, "unsupported_file_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			UploadHandler(h.svc, tc.max)(rec, tc.req, nil)

			assert.Equal(t, tc.status, rec.Code)
			var body apperrors.Response
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tc.code, body.Error.Code)
			assert.NotEmpty(t, body.Error.RequestID)
		})
	}
}
