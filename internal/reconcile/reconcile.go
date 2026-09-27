// Package reconcile decides whether a set of line items accounts for what the
// receipt says was paid. It is the single rule behind process, itemize and the
// user override, so all three agree on what "adds up" means.
package reconcile

// Status is the itemization outcome stored on a transaction.
type Status string

const (
	StatusComplete    Status = "COMPLETE"
	StatusNeedsReview Status = "NEEDS_REVIEW"
	StatusFailed      Status = "FAILED"
)

// Reason explains a status other than COMPLETE.
type Reason string

const (
	ReasonNone          Reason = ""
	ReasonTotalNotFound Reason = "total_not_found"
	ReasonNoLineItems   Reason = "no_line_items"
	ReasonTotalMismatch Reason = "total_mismatch"
)

// Tolerance absorbs a one-cent rounding difference between printed amounts.
const Tolerance int64 = 1

// Tax is the part of a tax line reconciliation needs.
type Tax struct {
	Amount    int64
	Inclusive bool
}

// Input is a receipt total, its taxes and a candidate set of item amounts.
type Input struct {
	GrandTotal  *int64
	Taxes       []Tax
	ItemAmounts []int64
}

// Result is the verdict plus the numbers behind it, so callers can show the
// gap instead of hiding it.
type Result struct {
	Status        Status
	Reason        Reason
	ExpectedTotal int64
	ItemsTotal    int64
	// TaxTotal counts exclusive taxes only; inclusive ones are already inside
	// the item amounts.
	TaxTotal int64
	// Difference is ExpectedTotal - (ItemsTotal + TaxTotal).
	Difference int64
}

// Reconciled reports whether the items account for the total.
func (r Result) Reconciled() bool { return r.Status == StatusComplete }

// Check applies Σ items + Σ exclusive taxes == grand total, within Tolerance.
// It never adjusts the total or proposes a balancing line.
func Check(in Input) Result {
	var res Result
	for _, amount := range in.ItemAmounts {
		res.ItemsTotal += amount
	}
	for _, tax := range in.Taxes {
		if !tax.Inclusive {
			res.TaxTotal += tax.Amount
		}
	}

	if in.GrandTotal == nil {
		res.Status, res.Reason = StatusFailed, ReasonTotalNotFound
		return res
	}

	res.ExpectedTotal = *in.GrandTotal
	res.Difference = res.ExpectedTotal - res.ItemsTotal - res.TaxTotal

	switch {
	case len(in.ItemAmounts) == 0:
		res.Status, res.Reason = StatusNeedsReview, ReasonNoLineItems
	case abs(res.Difference) > Tolerance:
		res.Status, res.Reason = StatusNeedsReview, ReasonTotalMismatch
	default:
		res.Status, res.Reason = StatusComplete, ReasonNone
	}
	return res
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
