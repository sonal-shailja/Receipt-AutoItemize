// Package money converts between decimal amounts as printed on receipts and
// int64 minor units. Every amount inside the service is minor units; the float
// conversions exist only for the JSON edge.
package money

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Scale is the number of minor units per major unit. Every supported currency
// is assumed to have two decimal places.
const Scale = 100

// ParseMinor parses a printed decimal such as "17.85", "3,50" or "24" into
// minor units without float arithmetic. A comma is read as the decimal mark.
func ParseMinor(s string) (int64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))

	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	whole, frac, hasFrac := strings.Cut(s, ".")
	if !digitsOnly(whole) || (hasFrac && (!digitsOnly(frac) || len(frac) > 2)) {
		return 0, fmt.Errorf("money: %q is not an amount with at most two decimals", s)
	}
	for len(frac) < 2 {
		frac += "0"
	}

	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("money: parsing %q: %w", s, err)
	}
	cents, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("money: parsing %q: %w", s, err)
	}

	minor := units*Scale + cents
	if negative {
		minor = -minor
	}
	return minor, nil
}

// ParseRateBps parses a printed percentage such as "19" or "7.5" into basis
// points. A percentage with two decimals scaled by 100 is exactly basis
// points, so this is ParseMinor under a clearer name.
func ParseRateBps(percent string) (int64, error) {
	return ParseMinor(percent)
}

// ToFloat renders minor units as a decimal number for JSON.
func ToFloat(minor int64) float64 {
	return float64(minor) / Scale
}

// FromFloat converts a JSON decimal into minor units, rejecting values with
// more than two decimals rather than silently rounding them.
func FromFloat(f float64) (int64, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("money: %v is not a finite amount", f)
	}
	scaled := f * Scale
	rounded := math.Round(scaled)
	if math.Abs(scaled-rounded) > 1e-6 {
		return 0, fmt.Errorf("money: %v has more than two decimals", f)
	}
	return int64(rounded), nil
}

// Format renders minor units as a fixed two-decimal string, e.g. "17.85".
func Format(minor int64) string {
	sign := ""
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	return fmt.Sprintf("%s%d.%02d", sign, minor/Scale, minor%Scale)
}

// RateToFraction renders basis points as a fraction: 1900 becomes 0.19.
func RateToFraction(bps int64) float64 {
	return float64(bps) / 10000
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
