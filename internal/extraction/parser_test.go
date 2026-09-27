package extraction

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../fixtures/task-a/" + name)
	require.NoError(t, err)
	return string(b)
}

func ptr(v int64) *int64 { return &v }

func TestParse_Clean(t *testing.T) {
	got := Parse(fixture(t, "receipt-clean.txt"))

	assert.Equal(t, "Cafe Mitte", got.Merchant)
	assert.Equal(t, "2026-03-12", got.Date)
	assert.Equal(t, "EUR", got.Currency)
	assert.Equal(t, ptr(1500), got.Subtotal)
	assert.Equal(t, ptr(1785), got.GrandTotal)
	assert.Equal(t, []Tax{{Name: "VAT", RateBps: 1900, Amount: 285}}, got.Taxes)
	assert.Equal(t, []Item{
		{Description: "Espresso", Amount: 350},
		{Description: "Sandwich", Amount: 890},
		{Description: "Mineral water", Amount: 260},
	}, got.Items)
}

func TestParse_TaxOnly(t *testing.T) {
	got := Parse(fixture(t, "receipt-tax-only.txt"))

	assert.Equal(t, "Berlin Taxi GmbH", got.Merchant)
	assert.Equal(t, ptr(2400), got.GrandTotal)
	assert.Nil(t, got.Subtotal)
	assert.Equal(t, []Tax{{Name: "VAT", RateBps: 1900, Amount: 383, Inclusive: true}}, got.Taxes)
	assert.Empty(t, got.Items, `"Trip fare" has no amount and must not become an item`)
}

func TestParse_Mismatch(t *testing.T) {
	got := Parse(fixture(t, "receipt-mismatch.txt"))

	assert.Equal(t, "Hotel Shop", got.Merchant)
	assert.Equal(t, "2026-03-13", got.Date)
	assert.Equal(t, ptr(1850), got.GrandTotal, "the printed total is kept even though items do not add up")
	assert.Equal(t, []Tax{{Name: "VAT", RateBps: 1900, Amount: 190}}, got.Taxes)
	assert.Equal(t, []Item{
		{Description: "Water", Amount: 400},
		{Description: "Snacks", Amount: 600},
	}, got.Items)
}

func TestParse_EdgeLines(t *testing.T) {
	got := Parse(`MERCHANT: Test Shop
Qty Item Price
Brot                2,50
Sub Total           2,50
MwSt 7%             0,18
total               2.68
Thank you!
(printed copy)
`)

	assert.Equal(t, []Item{{Description: "Brot", Amount: 250}}, got.Items)
	assert.Equal(t, ptr(250), got.Subtotal)
	assert.Equal(t, ptr(268), got.GrandTotal)
	assert.Equal(t, []Tax{{Name: "MwSt", RateBps: 700, Amount: 18}}, got.Taxes)
}

func TestParse_NoTotal(t *testing.T) {
	got := Parse("Espresso 3.50\n")
	assert.Nil(t, got.GrandTotal)
	assert.Len(t, got.Items, 1)
}

func TestParse_ColonLabelledSummaryLines(t *testing.T) {
	got := Parse("Espresso 15.00\nSubtotal: 15.00\nVAT 19%: 2.85\nTOTAL: 17.85\n")

	assert.Equal(t, []Item{{Description: "Espresso", Amount: 1500}}, got.Items)
	assert.Equal(t, ptr(1500), got.Subtotal)
	assert.Equal(t, ptr(1785), got.GrandTotal)
	assert.Equal(t, []Tax{{Name: "VAT", RateBps: 1900, Amount: 285}}, got.Taxes)
}

func TestParse_DiscountIsReceiptLevelAndPositive(t *testing.T) {
	cases := map[string]string{
		"printed with a minus": "Item 3.50\nDiscount -1.00\nTOTAL 2.50\n",
		"printed without sign": "Item 3.50\nDiscount 1.00\nTOTAL 2.50\n",
		"with a colon":         "Item 3.50\nDiscount: -1.00\nTOTAL 2.50\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			got := Parse(text)
			assert.Equal(t, []Item{{Description: "Item", Amount: 350}}, got.Items)
			assert.Equal(t, ptr(100), got.Discount)
		})
	}
}

func TestParse_OtherMinusLinesAreNotItems(t *testing.T) {
	// Item amounts are positive everywhere (PATCH rejects negatives too); a
	// minus line that is not a discount is ignored rather than guessed at.
	got := Parse("Item 3.50\nRefund -2.00\nTOTAL 3.50\n")

	assert.Equal(t, []Item{{Description: "Item", Amount: 350}}, got.Items)
	assert.Nil(t, got.Discount)
}
