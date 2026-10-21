// SPDX-License-Identifier: AGPL-3.0-or-later

package lua

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

var jsonNumber = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][-+]?[0-9]+)?$`)

// MarshalJSON writes the table as an object in source order. Index keys
// appear as "[n]". Numbers a float64 cannot hold (inf, nan, 1e999) become
// strings.
func (t *Table) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	t.writeJSON(&b)
	return b.Bytes(), nil
}

func (t *Table) writeJSON(b *bytes.Buffer) {
	b.WriteByte('{')
	for i, e := range t.Entries {
		if i > 0 {
			b.WriteByte(',')
		}
		writeString(b, e.Key.String())
		b.WriteByte(':')
		e.Value.writeJSON(b)
	}
	b.WriteByte('}')
}

func (v Value) writeJSON(b *bytes.Buffer) {
	switch v.Kind {
	case KindTable:
		v.Table.writeJSON(b)
	case KindString:
		writeString(b, v.Str)
	case KindBool:
		b.WriteString(strconv.FormatBool(v.Bool))
	case KindNumber:
		if isJSONNumber(v.Str) {
			b.WriteString(v.Str)
		} else {
			writeString(b, v.Str)
		}
	default:
		b.WriteString("null")
	}
}

// isJSONNumber reports whether s can be written as a bare JSON number that
// decodes to a float64.
func isJSONNumber(s string) bool {
	if !jsonNumber.MatchString(s) {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// writeString writes s as a JSON string, with unprintable runes that
// encoding/json leaves raw (C1 controls, bidi overrides) as \u escapes.
func writeString(b *bytes.Buffer, s string) {
	enc, _ := json.Marshal(s) //nolint:errchkjson // a string always marshals
	for len(enc) > 0 {
		r, n := utf8.DecodeRune(enc)
		if unicode.IsPrint(r) {
			b.Write(enc[:n])
		} else {
			for _, u := range utf16.Encode([]rune{r}) {
				fmt.Fprintf(b, `\u%04x`, u)
			}
		}
		enc = enc[n:]
	}
}
