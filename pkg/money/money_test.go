package money

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMinor(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"17.85", 1785, false},
		{"3,50", 350, false},
		{"24", 2400, false},
		{"2.6", 260, false},
		{"0.05", 5, false},
		{"-1.00", -100, false},
		{"", 0, true},
		{"1.234", 0, true},
		{"1.", 0, true},
		{"abc", 0, true},
		{"+5", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseMinor(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseRateBps(t *testing.T) {
	got, err := ParseRateBps("19")
	require.NoError(t, err)
	assert.Equal(t, int64(1900), got)

	got, err = ParseRateBps("7.5")
	require.NoError(t, err)
	assert.Equal(t, int64(750), got)
}

func TestFromFloat(t *testing.T) {
	got, err := FromFloat(17.85)
	require.NoError(t, err)
	assert.Equal(t, int64(1785), got)

	got, err = FromFloat(0.1 + 0.2)
	require.NoError(t, err)
	assert.Equal(t, int64(30), got)

	_, err = FromFloat(3.505)
	assert.Error(t, err, "more than two decimals")

	_, err = FromFloat(math.NaN())
	assert.Error(t, err)
}

func TestFormatAndFloats(t *testing.T) {
	assert.Equal(t, "17.85", Format(1785))
	assert.Equal(t, "0.05", Format(5))
	assert.Equal(t, "-6.60", Format(-660))
	assert.Equal(t, "0.00", Format(0))
	assert.Equal(t, 17.85, ToFloat(1785))
	assert.Equal(t, 0.19, RateToFraction(1900))
}

func TestAmountsBeyondRangeAreRejected(t *testing.T) {
	_, err := ParseMinor("99999999999999999")
	assert.Error(t, err, "would overflow int64 minor units")

	_, err = FromFloat(1e18)
	assert.Error(t, err, "would saturate int64 and let a sum wrap around")

	got, err := ParseMinor("10000000000000.00")
	require.NoError(t, err, "the largest supported amount still parses")
	assert.Equal(t, MaxMinor, got)
}
