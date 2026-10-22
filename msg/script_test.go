// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"reflect"
	"testing"
)

// scriptRec returns a script call record from the game with name and args.
func scriptRec(name, args string) []byte {
	p := make([]byte, 344)
	p[0], p[17] = TypeScriptCall, TypeScriptCall
	copy(p[24:], name)
	copy(p[152:], args)
	return p
}

func TestScriptCall(t *testing.T) {
	want := &ScriptCall{Name: "OnStreamComplete", Args: "1, true"}
	got := firstAt(fixtureMessages(t, "abyss-bot-278f.fwr"), 80, TypeScriptCall)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("abyss-bot-278f.fwr frame 80: %#v, want %#v", got, want)
	}
	if _, ok := got.(Command); ok {
		t.Error("ScriptCall is a Command")
	}

	dec := DefaultRegistry().Lookup(TypeScriptCall, 1383)
	leftover := scriptRec(want.Name, want.Args)
	copy(leftover[24+len(want.Name)+1:], "\xff\x01junk")
	copy(leftover[152+len(want.Args)+1:], "\x80\x1bjunk")
	if m, err := dec(leftover); err != nil || !reflect.DeepEqual(m, want) {
		t.Errorf("bytes after the NULs: %#v, %v", m, err)
	}
	if m, err := dec(scriptRec("", "")); err != nil || !reflect.DeepEqual(m, &ScriptCall{}) {
		t.Errorf("empty fields: %#v, %v", m, err)
	}

	set := func(p []byte, off int, v byte) []byte { p[off] = v; return p }
	fill := func(p []byte, lo, hi int) []byte {
		for i := lo; i < hi; i++ {
			p[i] = 'a'
		}
		return p
	}
	for name, p := range map[string][]byte{
		"no NUL in name": fill(scriptRec("", ""), 24, 152),
		"no NUL in args": fill(scriptRec("", ""), 152, 344),
		"0x80 in name":   set(scriptRec("OnStreamComplete", ""), 26, 0x80),
		"0x1b in args":   set(scriptRec("", "1, true"), 153, 0x1b),
		"0x7f in args":   set(scriptRec("", "1, true"), 152, 0x7f),
		"length 343":     scriptRec("OnStreamComplete", "1, true")[:343],
		"byte 17":        set(scriptRec("OnStreamComplete", "1, true"), 17, 0xe5),
	} {
		if m, err := dec(p); err == nil {
			t.Errorf("%s: decodes to %#v", name, m)
		}
	}
}
