package receipt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	receiptv1 "github.com/sonalshailja/receipt-autoitemize/rpc/receipt/v1"
)

func TestHandler_ProcessRendersGoldShape(t *testing.T) {
	h := newHarness(t)
	rec := h.upload(t, "receipt-tax-only.txt", "text/plain")
	handler := NewHandler(h.svc)

	got, err := handler.ProcessReceipt(context.Background(), &receiptv1.ProcessReceiptRequest{ReceiptId: rec.ID})
	require.NoError(t, err)

	assert.Equal(t, "Berlin Taxi GmbH", got.GetMerchant())
	assert.Equal(t, 24.0, got.GetGrandTotal())
	assert.Nil(t, got.Subtotal, "no subtotal printed, so none rendered")
	assert.Equal(t, "NEEDS_REVIEW", got.GetItemizeStatus())
	assert.Equal(t, "no_line_items", got.GetItemizeReason())
	assert.Empty(t, got.GetLineItems())
	require.Len(t, got.GetTaxes(), 1)
	assert.Equal(t, 0.19, got.GetTaxes()[0].GetRate())
	assert.Equal(t, 3.83, got.GetTaxes()[0].GetAmount())
	assert.True(t, got.GetTaxes()[0].GetInclusive())
}

func TestHandler_UpdateLineItems(t *testing.T) {
	h := newHarness(t)
	txn := h.process(t, "receipt-clean.txt")
	handler := NewHandler(h.svc)

	got, err := handler.UpdateLineItems(context.Background(), &receiptv1.UpdateLineItemsRequest{
		TransactionId: txn.ID,
		Items: []*receiptv1.LineItemInput{
			{Description: "Drinks", Amount: 6.10},
			{Description: "Sandwich", Amount: 8.90},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "COMPLETE", got.GetItemizeStatus())
	assert.Equal(t, "USER", got.GetLineItems()[0].GetSource())

	_, err = handler.UpdateLineItems(context.Background(), &receiptv1.UpdateLineItemsRequest{
		TransactionId: txn.ID,
		Items:         []*receiptv1.LineItemInput{{Description: "Drinks", Amount: 3.505}},
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = handler.UpdateLineItems(context.Background(), &receiptv1.UpdateLineItemsRequest{
		TransactionId: txn.ID,
		Items:         []*receiptv1.LineItemInput{{Description: "Drinks", Amount: 1}},
	})
	assert.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestHandler_GetUnknownIsNotFound(t *testing.T) {
	_, err := NewHandler(newHarness(t).svc).GetTransaction(context.Background(),
		&receiptv1.GetTransactionRequest{TransactionId: "txn_missing"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}
