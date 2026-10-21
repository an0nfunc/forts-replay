// SPDX-License-Identifier: AGPL-3.0-or-later

package lua

import (
	"errors"
	"math"
	"strings"
	"testing"
)

const sample = "data = \r\n{\r\n\tDoorsOpened = false,\r\n\tScheduledCalls = \r\n\t{\r\n\t},\r\n" +
	"\tgameFrame = 1,\r\n\tgameTime = 0.03999999910593,\r\n\tupdateDelta = 0.03999999910593,\r\n}\r\n\r\n\n"

func mustParse(t *testing.T, src string) *Table {
	t.Helper()
	tbl, err := Parse([]byte(src), Limits{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return tbl
}

func get(t *testing.T, tbl *Table, path ...string) Value {
	t.Helper()
	var v Value
	for _, p := range path {
		var ok bool
		if v, ok = tbl.Get(p); !ok {
			t.Fatalf("missing %s", strings.Join(path, "."))
		}
		tbl = v.Table
	}
	return v
}

func TestParseSample(t *testing.T) {
	root := mustParse(t, sample)
	if v := get(t, root, "data", "gameFrame"); v.Kind != KindNumber || v.Str != "1" {
		t.Errorf("gameFrame %+v", v)
	}
	if v := get(t, root, "data", "gameTime"); v.Str != "0.03999999910593" {
		t.Errorf("gameTime %+v", v)
	}
	if v := get(t, root, "data", "DoorsOpened"); v.Kind != KindBool || v.Bool {
		t.Errorf("DoorsOpened %+v", v)
	}
	if v := get(t, root, "data", "ScheduledCalls"); v.Kind != KindTable || len(v.Table.Entries) != 0 {
		t.Errorf("ScheduledCalls %+v", v)
	}
}

func TestParse(t *testing.T) {
	src := "data = \r\n{\r\n" +
		"\t-- callback = '!!!FUNCTION!!! OnUpdate 12'\r\n" +
		"\tweapons = \r\n\t{\r\n\t\t\"flak\",\r\n\t\t\"laser\",\r\n\t},\r\n" +
		"\t[251] = \r\n\t{\r\n\t\t[0] = -10,\r\n\t\tx = -2.5,\r\n\t},\r\n" +
		"\tbig = 1e+20,\r\n\tpos = inf,\r\n\tneg = -inf,\r\n\tbad = -nan(ind),\r\n\tq = nan,\r\n" +
		"\twide = L\"caf\xc3\xa9\",\r\n\tpath = \"maps\\Abyss\",\r\n\tok = true,\r\n" +
		"\t[\"spaced key\"] = 1,\r\n}\r\n"
	root := mustParse(t, src)
	data := get(t, root, "data").Table
	if len(data.Entries) != 11 {
		t.Fatalf("%d entries, the comment line must be skipped", len(data.Entries))
	}
	w := get(t, root, "data", "weapons").Table
	if w.Entries[0].Key != (Key{Index: 1, IsIndex: true}) || w.Entries[1].Value.Str != "laser" {
		t.Errorf("positional entries %+v", w.Entries)
	}
	idx := data.Entries[1]
	if idx.Key != (Key{Index: 251, IsIndex: true}) || idx.Value.Table.Entries[0].Key != (Key{Index: 0, IsIndex: true}) {
		t.Errorf("index keys %+v", idx)
	}
	floats := map[string]float64{"big": 1e20, "pos": math.Inf(1), "neg": math.Inf(-1)}
	for name, want := range floats {
		if f, ok := get(t, root, "data", name).Float(); !ok || f != want {
			t.Errorf("%s = %v", name, f)
		}
	}
	for _, name := range []string{"bad", "q"} {
		if f, ok := get(t, root, "data", name).Float(); !ok || !math.IsNaN(f) {
			t.Errorf("%s = %v, want NaN", name, f)
		}
	}
	if v := get(t, root, "data", "wide"); v.Kind != KindString || v.Str != "café" {
		t.Errorf("wide %+v", v)
	}
	if v := get(t, root, "data", "path"); v.Str != `maps\Abyss` {
		t.Errorf("backslash must stay raw: %q", v.Str)
	}
	if v := get(t, root, "data", "spaced key"); v.Str != "1" {
		t.Errorf("bracketed string key %+v", v)
	}
	if semi := get(t, mustParse(t, "data = { a = 1; b = 2; }"), "data").Table; len(semi.Entries) != 2 {
		t.Errorf("';' separators: %+v", semi.Entries)
	}
}

func TestDepthLimit(t *testing.T) {
	nest := func(n int) string {
		return "data = " + strings.Repeat("{", n) + strings.Repeat("}", n)
	}
	if _, err := Parse([]byte(nest(64)), Limits{}); err != nil {
		t.Errorf("depth 64: %v", err)
	}
	if _, err := Parse([]byte(nest(65)), Limits{}); !errors.Is(err, ErrLimit) {
		t.Errorf("depth 65: %v", err)
	}
	if _, err := Parse([]byte(nest(100000)), Limits{}); !errors.Is(err, ErrLimit) {
		t.Errorf("depth 100000: %v", err)
	}
}

func TestSizeLimit(t *testing.T) {
	if _, err := Parse([]byte(sample), Limits{MaxSize: 10}); !errors.Is(err, ErrLimit) {
		t.Errorf("err %v", err)
	}
}

func TestSyntaxErrors(t *testing.T) {
	for _, src := range []string{
		"data = {",
		"data = { x = 1",
		"data = { x = }",
		"data = { foo bar = 1 }",
		"data = { s = \"unterminated }",
		"data = { s = \"has \"quote\" inside\", }",
		"data = { n = 1.2.3, }",
		"data = { n = - , }",
		"= 1",
		"data = { [1 = 2 }",
		"data = { x = @ }",
		"data = { x = foo }",
	} {
		_, err := Parse([]byte(src), Limits{})
		if !errors.Is(err, ErrSyntax) {
			t.Errorf("%q: err %v, want ErrSyntax", src, err)
		}
	}
}

func TestSyntaxErrorLine(t *testing.T) {
	_, err := Parse([]byte("data = \r\n{\r\n\tx = 1,\r\n\ty = ?,\r\n}"), Limits{})
	if err == nil || !strings.Contains(err.Error(), "line 4") {
		t.Errorf("err %v, want line 4", err)
	}
}
