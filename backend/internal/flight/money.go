package flight

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Money is an amount in minor currency units (cents). Holding prices as
// integers keeps sums exact; floating point would drift.
type Money int64

// maxWholeDigits bounds the whole part of a parsed amount so the conversion
// to cents cannot overflow an int64.
const maxWholeDigits = 15

// ErrInvalidMoney is returned when text cannot be read as an amount.
var ErrInvalidMoney = errors.New("invalid amount")

// ParseMoney reads a non-negative decimal amount such as "299", "299.5" or
// "299.00". At most two fractional digits are accepted; signs, exponents,
// thousands separators and currency symbols are rejected rather than guessed
// at.
func ParseMoney(s string) (Money, error) {
	s = strings.TrimSpace(s)
	whole, frac, hasFrac := strings.Cut(s, ".")

	if whole == "" || len(whole) > maxWholeDigits || !allDigits(whole) {
		return 0, fmt.Errorf("%w: %q", ErrInvalidMoney, s)
	}
	if hasFrac && (frac == "" || len(frac) > 2 || !allDigits(frac)) {
		return 0, fmt.Errorf("%w: %q", ErrInvalidMoney, s)
	}

	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidMoney, s)
	}

	var cents int64
	switch len(frac) {
	case 1:
		cents = int64(frac[0]-'0') * 10
	case 2:
		cents = int64(frac[0]-'0')*10 + int64(frac[1]-'0')
	}
	return Money(units*100 + cents), nil
}

// String formats the amount with exactly two decimals, e.g. "299.00".
func (m Money) String() string {
	sign := ""
	if m < 0 {
		sign = "-"
		m = -m
	}
	return fmt.Sprintf("%s%d.%02d", sign, int64(m)/100, int64(m)%100)
}

// MarshalJSON writes the amount as a JSON number with two decimals.
func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(m.String()), nil
}

// UnmarshalJSON accepts either a JSON number (299.00) or a JSON string
// holding a decimal ("299.00"). Source data is not consistent about which it
// uses.
func (m *Money) UnmarshalJSON(data []byte) error {
	text := string(data)
	if len(data) > 0 && data[0] == '"' {
		unquoted, err := strconv.Unquote(text)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidMoney, text)
		}
		text = unquoted
	}
	parsed, err := ParseMoney(text)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
