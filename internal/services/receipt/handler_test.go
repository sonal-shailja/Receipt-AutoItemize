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

func TestHandler_RendersDiscountOnlyWhenPrinted(t *testing.T) {
	h := newHarness(t)
	handler := NewHandler(h.svc)

	clean := h.upload(t, "receipt-clean.txt", "text/plain")
	got, err := handler.ProcessReceipt(context.Background(), &receiptv1.ProcessReceiptRequest{ReceiptId: clean.ID})
	require.NoError(t, err)
	assert.Nil(t, got.Discount)

	discounted := h.uploadText(t, "discount.txt", "Item 3.50\nDiscount -1.00\nTOTAL 2.50\n")
	got, err = handler.ProcessReceipt(context.Background(), &receiptv1.ProcessReceiptRequest{ReceiptId: discounted.ID})
	require.NoError(t, err)
	assert.Equal(t, 1.0, got.GetDiscount())
	assert.Equal(t, "COMPLETE", got.GetItemizeStatus())
}

func TestHandler_UpdateLineItems_QuantityAndTaxAmountRoundTrip(t *testing.T) {
	h := newHarness(t)
	txn := h.process(t, "receipt-clean.txt")
	handler := NewHandler(h.svc)
	two := int32(2)
	itemTax := 0.50

	got, err := handler.UpdateLineItems(context.Background(), &receiptv1.UpdateLineItemsRequest{
		TransactionId: txn.ID,
		Items: []*receiptv1.LineItemInput{
			{Description: "Drinks", Amount: 6.10, Quantity: &two, TaxAmount: &itemTax},
			{Description: "Sandwich", Amount: 8.90},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "COMPLETE", got.GetItemizeStatus(), "item tax_amount is informational, not added to the total")
	assert.Equal(t, int32(2), got.GetLineItems()[0].GetQuantity())
	assert.Equal(t, 0.50, got.GetLineItems()[0].GetTaxAmount())
	assert.Nil(t, got.GetLineItems()[1].Quantity)
	assert.Nil(t, got.GetLineItems()[1].TaxAmount)
}

func TestHandler_UpdateLineItems_RejectsUnrepresentableAmounts(t *testing.T) {
	h := newHarness(t)
	txn := h.process(t, "receipt-clean.txt")
	handler := NewHandler(h.svc)
	badTax := 0.505

	cases := map[string][]*receiptv1.LineItemInput{
		"tax amount with three decimals": {{Description: "Drinks", Amount: 17.85, TaxAmount: &badTax}},
		// Two 1e18 items would overflow int64 and wrap the sum onto the total.
		"amount beyond the supported range": {
			{Description: "a", Amount: 1e18}, {Description: "b", Amount: 1e18}, {Description: "c", Amount: 17.87},
		},
	}
	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := handler.UpdateLineItems(context.Background(), &receiptv1.UpdateLineItemsRequest{TransactionId: txn.ID, Items: items})
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}
