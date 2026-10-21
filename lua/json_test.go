// SPDX-License-Identifier: AGPL-3.0-or-later

package lua

import (
	"encoding/json"
	"testing"
)

func TestJSON(t *testing.T) {
	root := mustParse(t, "data = { a = 1, b = -2.5, c = \"a\\b\", [3] = true, \"pos\", i = inf, n = -nan(ind), e = 1e+20, t = {}, }")
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"data":{"a":1,"b":-2.5,"c":"a\\b","[3]":true,"[1]":"pos","i":"inf","n":"-nan(ind)","e":1e+20,"t":{}}}`
	if string(out) != want {
		t.Errorf("got  %s\nwant %s", out, want)
	}
	if !json.Valid(out) {
		t.Error("invalid JSON")
	}
}

func TestJSONEdgeCases(t *testing.T) {
	root := mustParse(t, "data = { big = 1e999, tiny = 1e-999, s = \"\u009b\u202e\", }")
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"data":{"big":"1e999","tiny":1e-999,"s":"\u009b\u202e"}}`
	if string(out) != want {
		t.Errorf("got  %s\nwant %s", out, want)
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Errorf("decode: %v", err)
	}
}
