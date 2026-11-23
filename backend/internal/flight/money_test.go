package flight

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseMoney(t *testing.T) {
	valid := []struct {
		in   string
		want Money
	}{
		{"0", 0},
		{"99", 9900},
		{"299.00", 29900},
		{"299.5", 29950},
		{"299.05", 29905},
		{"  109 ", 10900},
		{"0.01", 1},
	}
	for _, tc := range valid {
		got, err := ParseMoney(tc.in)
		if err != nil {
			t.Errorf("ParseMoney(%q) returned error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseMoney(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}

	invalid := []string{"", " ", "abc", "-5", "+5", "1.234", "1.", ".5", "1e3", "1,000", "$5", "12.3.4", "1234567890123456"}
	for _, in := range invalid {
		if _, err := ParseMoney(in); !errors.Is(err, ErrInvalidMoney) {
			t.Errorf("ParseMoney(%q) error = %v, want ErrInvalidMoney", in, err)
		}
	}
}

func TestMoneyString(t *testing.T) {
	cases := []struct {
		in   Money
		want string
	}{
		{0, "0.00"},
		{5, "0.05"},
		{29900, "299.00"},
		{41450, "414.50"},
		{-150, "-1.50"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("Money(%d).String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMoneyJSON(t *testing.T) {
	// The source data mixes numbers and numeric strings for the same field.
	var decoded struct {
		Number  Money `json:"number"`
		Integer Money `json:"integer"`
		Text    Money `json:"text"`
	}
	input := `{"number": 299.00, "integer": 109, "text": "95.00"}`
	if err := json.Unmarshal([]byte(input), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Number != 29900 || decoded.Integer != 10900 || decoded.Text != 9500 {
		t.Errorf("decoded = %+v, want 29900, 10900, 9500", decoded)
	}

	encoded, err := json.Marshal(struct {
		Price Money `json:"price"`
	}{Price: 41400})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := `{"price":414.00}`; string(encoded) != want {
		t.Errorf("marshal = %s, want %s", encoded, want)
	}

	for _, bad := range []string{`{"number": "abc"}`, `{"number": -1}`, `{"number": null}`, `{"number": true}`} {
		if err := json.Unmarshal([]byte(bad), &decoded); err == nil {
			t.Errorf("unmarshal %s succeeded, want error", bad)
		}
	}
}

func TestMoneySumsAreExact(t *testing.T) {
	// 0.10 + 0.20 is the classic floating point failure; cents add exactly.
	a, _ := ParseMoney("0.10")
	b, _ := ParseMoney("0.20")
	if got := (a + b).String(); got != "0.30" {
		t.Errorf("0.10 + 0.20 = %s, want 0.30", got)
	}
}
