// Package extraction turns raw OCR text into a structured receipt. It is a pure
// function of the text, so re-itemizing replays it over stored OCR output
// without touching the file or the OCR provider again.
package extraction

import (
	"regexp"
	"strings"

	"github.com/sonalshailja/receipt-autoitemize/pkg/money"
)

// Tax is one tax line exactly as printed.
type Tax struct {
	Name    string
	RateBps int64
	Amount  int64
	// Inclusive is true for "incl. VAT" lines: the tax is already inside the total.
	Inclusive bool
}

// Item is one proposed line item.
type Item struct {
	Description string
	Amount      int64
}

// Receipt is everything Parse could read. Missing values stay zero or nil;
// deciding what that means is the reconciler's job, not the parser's.
type Receipt struct {
	Merchant   string
	Date       string
	Currency   string
	Subtotal   *int64
	GrandTotal *int64
	// Discount is the receipt-level discount, always positive however it is
	// printed; it is subtracted when reconciling.
	Discount *int64
	Taxes    []Tax
	Items    []Item
}

var (
	// headerLine is "KEY: value".
	headerLine = regexp.MustCompile(`^([A-Za-z]+):\s*(.+)$`)
	// amountLine is a label followed by an amount at the end of the line.
	amountLine = regexp.MustCompile(`^(.*\S)\s+(-?\d+(?:[.,]\d{1,2})?)$`)
	// taxLabel is "VAT 19%" or "incl. VAT 19%".
	taxLabel = regexp.MustCompile(`(?i)^(incl\.?\s+)?([a-z]+)\s+(\d+(?:[.,]\d{1,2})?)\s*%$`)
)

var (
	totalLabels    = map[string]bool{"total": true, "grand total": true, "amount due": true}
	subtotalLabels = map[string]bool{"subtotal": true, "sub total": true, "sub-total": true}
	discountLabels = map[string]bool{"discount": true, "rabatt": true}
)

// Parse reads header fields, summary lines, taxes and items from OCR text.
//
// Summary lines (subtotal, total, discount) and tax lines end in an amount but
// are not items. Item amounts are positive, so any other minus line is ignored
// rather than guessed at. Lines without an amount — a service description such as "Trip fare",
// column headers, greetings — and parenthesised notes are ignored: turning
// them into items would invent data.
func Parse(text string) Receipt {
	var r Receipt

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || isNote(line) || parseHeader(line, &r) {
			continue
		}

		m := amountLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// "TOTAL: 17.85" and "TOTAL 17.85" are the same line.
		label := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(m[1]), ":"))
		amount, err := money.ParseMinor(m[2])
		if err != nil {
			continue
		}

		lower := strings.ToLower(label)
		switch {
		case totalLabels[lower]:
			r.GrandTotal = &amount
		case subtotalLabels[lower]:
			r.Subtotal = &amount
		case discountLabels[lower]:
			discount := abs(amount)
			if r.Discount != nil {
				discount += *r.Discount
			}
			r.Discount = &discount
		case amount < 0:
			continue
		default:
			if tax, ok := parseTax(label, amount); ok {
				r.Taxes = append(r.Taxes, tax)
				continue
			}
			r.Items = append(r.Items, Item{Description: label, Amount: amount})
		}
	}

	return r
}

func isNote(line string) bool {
	return strings.HasPrefix(line, "(") && strings.HasSuffix(line, ")")
}

// parseHeader fills a known header field and reports whether line was one.
func parseHeader(line string, r *Receipt) bool {
	m := headerLine.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	value := strings.TrimSpace(m[2])
	switch strings.ToUpper(m[1]) {
	case "MERCHANT":
		r.Merchant = value
	case "DATE":
		r.Date = value
	case "CURRENCY":
		r.Currency = strings.ToUpper(value)
	default:
		return false
	}
	return true
}

func parseTax(label string, amount int64) (Tax, bool) {
	m := taxLabel.FindStringSubmatch(label)
	if m == nil {
		return Tax{}, false
	}
	rate, err := money.ParseRateBps(m[3])
	if err != nil {
		return Tax{}, false
	}
	return Tax{Name: m[2], RateBps: rate, Amount: amount, Inclusive: m[1] != ""}, true
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
