// Package replyfield keeps a toast's reply buttons in one record field.
//
// A record's fields are strings, so the buttons are stored as a JSON array.
// They were once joined with " | ", which a label containing " | " broke
// apart on the way back; Decode still reads that form, since records are
// never rewritten.
package replyfield

import (
	"encoding/json"
	"strings"
)

// Encode returns the field value for these buttons: "" for none.
func Encode(buttons []string) string {
	if len(buttons) == 0 {
		return ""
	}
	b, err := json.Marshal(buttons)
	if err != nil { // a []string always marshals
		return ""
	}
	return string(b)
}

// Decode reads a field Encode wrote, or the older " | " form.
func Decode(field string) []string {
	if field == "" {
		return nil
	}
	if strings.HasPrefix(field, "[") {
		var out []string
		if json.Unmarshal([]byte(field), &out) == nil {
			return out
		}
	}
	var out []string
	for _, s := range strings.Split(field, " | ") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
