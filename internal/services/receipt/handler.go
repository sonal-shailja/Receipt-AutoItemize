package receipt

import (
	"context"

	transactionentity "github.com/sonalshailja/receipt-autoitemize/internal/entities/transaction"
	apperrors "github.com/sonalshailja/receipt-autoitemize/internal/errors"
	"github.com/sonalshailja/receipt-autoitemize/pkg/money"
	receiptv1 "github.com/sonalshailja/receipt-autoitemize/rpc/receipt/v1"
)

// Handler adapts Service to the generated gRPC interface.
type Handler struct {
	receiptv1.UnimplementedReceiptServiceServer

	service *Service
}

// NewHandler builds a Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ProcessReceipt implements receiptv1.ReceiptServiceServer.
func (h *Handler) ProcessReceipt(ctx context.Context, req *receiptv1.ProcessReceiptRequest) (*receiptv1.Transaction, error) {
	txn, err := h.service.Process(ctx, req.GetReceiptId())
	if err != nil {
		return nil, apperrors.ToGRPCStatus(err)
	}
	return toProto(txn), nil
}

// GetTransaction implements receiptv1.ReceiptServiceServer.
func (h *Handler) GetTransaction(ctx context.Context, req *receiptv1.GetTransactionRequest) (*receiptv1.Transaction, error) {
	txn, err := h.service.Get(ctx, req.GetTransactionId())
	if err != nil {
		return nil, apperrors.ToGRPCStatus(err)
	}
	return toProto(txn), nil
}

// ItemizeTransaction implements receiptv1.ReceiptServiceServer.
func (h *Handler) ItemizeTransaction(ctx context.Context, req *receiptv1.ItemizeTransactionRequest) (*receiptv1.Transaction, error) {
	txn, err := h.service.Itemize(ctx, req.GetTransactionId())
	if err != nil {
		return nil, apperrors.ToGRPCStatus(err)
	}
	return toProto(txn), nil
}

// UpdateLineItems implements receiptv1.ReceiptServiceServer.
func (h *Handler) UpdateLineItems(ctx context.Context, req *receiptv1.UpdateLineItemsRequest) (*receiptv1.Transaction, error) {
	inputs := make([]ItemInput, 0, len(req.GetItems()))
	for i, item := range req.GetItems() {
		amount, err := money.FromFloat(item.GetAmount())
		if err != nil {
			return nil, apperrors.ToGRPCStatus(apperrors.Newf(apperrors.KindInvalidArgument,
				"item_amount_invalid", "items[%d].amount: %v", i, err))
		}
		in := ItemInput{Description: item.GetDescription(), AmountMinor: amount, Quantity: item.Quantity}
		if item.TaxAmount != nil {
			taxAmount, err := money.FromFloat(item.GetTaxAmount())
			if err != nil {
				return nil, apperrors.ToGRPCStatus(apperrors.Newf(apperrors.KindInvalidArgument,
					"item_tax_amount_invalid", "items[%d].tax_amount: %v", i, err))
			}
			in.TaxAmountMinor = &taxAmount
		}
		inputs = append(inputs, in)
	}

	txn, err := h.service.UpdateItems(ctx, req.GetTransactionId(), inputs)
	if err != nil {
		return nil, apperrors.ToGRPCStatus(err)
	}
	return toProto(txn), nil
}

func toProto(txn *transactionentity.Transaction) *receiptv1.Transaction {
	out := &receiptv1.Transaction{
		Id:            txn.ID,
		ReceiptId:     txn.ReceiptID,
		OcrResultId:   txn.OCRResultID,
		Merchant:      txn.Merchant,
		Date:          txn.Date,
		Currency:      txn.Currency,
		Subtotal:      optionalAmount(txn.SubtotalMinor),
		GrandTotal:    optionalAmount(txn.GrandTotalMinor),
		ItemizeStatus: string(txn.ItemizeStatus),
		ItemizeReason: string(txn.ItemizeReason),
		Taxes:         make([]*receiptv1.Tax, 0, len(txn.Taxes)),
		LineItems:     make([]*receiptv1.LineItem, 0, len(txn.LineItems)),
	}
	for _, tax := range txn.Taxes {
		out.Taxes = append(out.Taxes, &receiptv1.Tax{
			Id:        tax.ID,
			Name:      tax.Name,
			Rate:      money.RateToFraction(tax.RateBps),
			Amount:    money.ToFloat(tax.AmountMinor),
			Inclusive: tax.Inclusive,
		})
	}
	for _, item := range txn.LineItems {
		out.LineItems = append(out.LineItems, &receiptv1.LineItem{
			Id:          item.ID,
			Description: item.Description,
			Amount:      money.ToFloat(item.AmountMinor),
			Quantity:    item.Quantity,
			TaxAmount:   optionalAmount(item.TaxAmountMinor),
			Source:      string(item.Source),
		})
	}
	return out
}

func optionalAmount(minor *int64) *float64 {
	if minor == nil {
		return nil
	}
	value := money.ToFloat(*minor)
	return &value
}
