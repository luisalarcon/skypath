package dataset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// record is one element of a record array, decoded only as far as its keys.
//
// Decoding into a map rather than a struct keeps key matching exact (struct
// decoding would also accept "Code" or "CODE" for "code") and lets each field
// be checked on its own, so a reason can tell a missing field from one of the
// wrong type.
type record map[string]json.RawMessage

func decodeRecord(raw json.RawMessage) (record, error) {
	// A null would decode into a nil map without error, so the kind of value
	// has to be checked first.
	if !isKind(raw, '{') {
		return nil, fmt.Errorf("record is %s, not a JSON object", kindOf(raw))
	}
	var r record
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("record cannot be decoded: %v", err)
	}
	return r, nil
}

// has reports whether key is present with a value other than null.
func (r record) has(key string) bool {
	raw, ok := r[key]
	return ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// optionalText returns the string value of key exactly as given, or "" when
// the key is absent or null. Any other kind of value is an error.
func (r record) optionalText(key, name string) (string, error) {
	if !r.has(key) {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(r[key], &s); err != nil {
		return "", fmt.Errorf("%s is %s, not a string", name, kindOf(r[key]))
	}
	return s, nil
}

// requiredText returns the string value of key with surrounding space
// removed. Absent, null and blank values are all errors.
func (r record) requiredText(key, name string) (string, error) {
	if !r.has(key) {
		return "", fmt.Errorf("missing %s", name)
	}
	s, err := r.optionalText(key, name)
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty %s", name)
	}
	return s, nil
}

// isKind reports whether the JSON value starts with the given delimiter.
func isKind(raw json.RawMessage, delim byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == delim
}

// kindOf names the kind of a JSON value for use in a reason.
func kindOf(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "empty"
	}
	switch trimmed[0] {
	case '{':
		return "an object"
	case '[':
		return "an array"
	case '"':
		return "a string"
	case 't', 'f':
		return "a boolean"
	case 'n':
		return "null"
	default:
		return "a number"
	}
}
