package boot

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// readAll reports whether the wrapped handler could read the whole body.
var readAll = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if _, err := io.ReadAll(r.Body); err != nil {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	w.WriteHeader(http.StatusOK)
})

func TestLimitJSONBodies(t *testing.T) {
	handler := limitJSONBodies(readAll, 16)
	big := strings.Repeat("x", 64)

	cases := []struct {
		name   string
		path   string
		body   string
		status int
	}{
		{"small JSON body passes", "/transactions/txn_1/items", "{}", http.StatusOK},
		{"oversized JSON body is cut off", "/transactions/txn_1/items", big, http.StatusRequestEntityTooLarge},
		{"upload keeps its own larger limit", "/receipts", big, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, tc.path, strings.NewReader(tc.body)))
			assert.Equal(t, tc.status, rec.Code)
		})
	}
}
