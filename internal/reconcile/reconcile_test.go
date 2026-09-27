package reconcile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func total(v int64) *int64 { return &v }

func TestCheck(t *testing.T) {
	cases := []struct {
		name   string
		in     Input
		status Status
		reason Reason
		diff   int64
	}{
		{"clean receipt reconciles",
			Input{GrandTotal: total(1785), Taxes: []Tax{{Amount: 285}}, ItemAmounts: []int64{350, 890, 260}},
			StatusComplete, ReasonNone, 0},
		{"mismatch keeps the gap visible",
			Input{GrandTotal: total(1850), Taxes: []Tax{{Amount: 190}}, ItemAmounts: []int64{400, 600}},
			StatusNeedsReview, ReasonTotalMismatch, 660},
		{"no items needs review",
			Input{GrandTotal: total(2400), Taxes: []Tax{{Amount: 383, Inclusive: true}}},
			StatusNeedsReview, ReasonNoLineItems, 2400},
		{"inclusive tax is not added on top",
			Input{GrandTotal: total(2400), Taxes: []Tax{{Amount: 383, Inclusive: true}}, ItemAmounts: []int64{2400}},
			StatusComplete, ReasonNone, 0},
		{"one cent rounding is tolerated",
			Input{GrandTotal: total(1786), Taxes: []Tax{{Amount: 285}}, ItemAmounts: []int64{350, 890, 260}},
			StatusComplete, ReasonNone, 1},
		{"two cents is a mismatch",
			Input{GrandTotal: total(1787), Taxes: []Tax{{Amount: 285}}, ItemAmounts: []int64{350, 890, 260}},
			StatusNeedsReview, ReasonTotalMismatch, 2},
		{"missing total fails",
			Input{ItemAmounts: []int64{350}},
			StatusFailed, ReasonTotalNotFound, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Check(tc.in)
			assert.Equal(t, tc.status, got.Status)
			assert.Equal(t, tc.reason, got.Reason)
			assert.Equal(t, tc.diff, got.Difference)
			assert.Equal(t, tc.status == StatusComplete, got.Reconciled())
		})
	}
}

func TestCheck_Totals(t *testing.T) {
	got := Check(Input{
		GrandTotal:  total(1850),
		Taxes:       []Tax{{Amount: 190}, {Amount: 50, Inclusive: true}},
		ItemAmounts: []int64{400, 600},
	})
	assert.Equal(t, int64(1850), got.ExpectedTotal)
	assert.Equal(t, int64(1000), got.ItemsTotal)
	assert.Equal(t, int64(190), got.TaxTotal, "inclusive taxes are excluded")
}
