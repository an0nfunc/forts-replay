// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/an0nfunc/forts-replay/internal/corpus"
	"github.com/an0nfunc/forts-replay/internal/fwrtest"
)

func readAll(t *testing.T, data []byte, lim Limits) (Header, []Block, error) {
	t.Helper()
	r, err := NewReader(bytes.NewReader(data), lim)
	if err != nil {
		return Header{}, nil, err
	}
	var blocks []Block
	for r.NextBlock() {
		blocks = append(blocks, r.Block())
	}
	return r.Header(), blocks, r.Err()
}

func TestFixtures(t *testing.T) {
	cases := []struct {
		file   string
		frames uint32
		sizes  []int
	}{
		{"abyss-bot-278f.fwr", 278, []int{78358, 79167, 65940}},
		{"abyss-bot-974f.fwr", 974, []int{79311, 79194, 78532, 78285, 77734, 77814, 76488, 66760}},
	}
	for _, c := range cases {
		data, err := os.ReadFile(corpus.Path(t, "..", "v1.38.2/"+c.file))
		if err != nil {
			t.Fatal(err)
		}
		h, blocks, err := readAll(t, data, Limits{})
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if h.Version != 1383 || h.Frames != c.frames || h.FirstBlock != 0x797 || len(h.Raw) != 0x797 {
			t.Errorf("%s: header %+v", c.file, h)
		}
		var sizes []int
		for i, b := range blocks {
			if b.Index != i {
				t.Errorf("%s: block %d has index %d", c.file, i, b.Index)
			}
			sizes = append(sizes, len(b.Data))
		}
		if !slices.Equal(sizes, c.sizes) {
			t.Errorf("%s: block sizes %v, want %v", c.file, sizes, c.sizes)
		}
		if blocks[0].Offset != 0x797 {
			t.Errorf("%s: first block offset %#x", c.file, blocks[0].Offset)
		}
	}
}

func TestSynthetic(t *testing.T) {
	big := bytes.Repeat([]byte("forts"), 3000)
	data := fwrtest.File(fwrtest.Header{Version: 7, Frames: 25, Unknown9: 2, Extra: make([]byte, 10)}, []byte("hello"), big)
	h, blocks, err := readAll(t, data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != 7 || h.Frames != 25 || h.Unknown9 != 2 || h.FirstBlock != 27 {
		t.Errorf("header %+v", h)
	}
	if len(blocks) != 2 || string(blocks[0].Data) != "hello" || !bytes.Equal(blocks[1].Data, big) {
		t.Fatalf("blocks %d", len(blocks))
	}
	if blocks[1].Offset != 27+12+int64(blocks[0].CompressedSize) {
		t.Errorf("second block offset %d", blocks[1].Offset)
	}
}

func TestTruncated(t *testing.T) {
	full := fwrtest.File(fwrtest.Header{Version: 1}, []byte("one"), []byte("two"))
	firstEnd := 17 + len(fwrtest.Block([]byte("one")))
	cases := []struct {
		name   string
		cut    int
		blocks int
	}{
		{"inside fixed header", 10, 0},
		{"no blocks", 17, 0},
		{"inside first block header", 17 + 6, 0},
		{"inside first block payload", 17 + 20, 0},
		{"inside second block header", firstEnd + 4, 1},
		{"inside second block payload", len(full) - 3, 1},
	}
	for _, c := range cases {
		_, blocks, err := readAll(t, full[:c.cut], Limits{})
		if !errors.Is(err, ErrTruncated) {
			t.Errorf("%s: err %v, want ErrTruncated", c.name, err)
		}
		if len(blocks) != c.blocks {
			t.Errorf("%s: %d blocks, want %d", c.name, len(blocks), c.blocks)
		}
	}
}

func TestCorrupt(t *testing.T) {
	good := fwrtest.File(fwrtest.Header{Version: 1}, []byte("payload"))
	mutate := func(f func(b []byte)) []byte {
		b := slices.Clone(good)
		f(b)
		return b
	}
	cases := map[string][]byte{
		"block marker": mutate(func(b []byte) { b[17] ^= 0xff }),
		"inner size":   mutate(func(b []byte) { b[17+12] ^= 0x01 }),
		"lzma stream": mutate(func(b []byte) {
			for i := 17 + 21; i < len(b); i++ {
				b[i] = 0xff
			}
		}),
		"lzma props":          mutate(func(b []byte) { b[17+16] = 0xff }),
		"first block < 17":    mutate(func(b []byte) { b[13] = 5 }),
		"compressed size < 9": mutate(func(b []byte) { binary.LittleEndian.PutUint32(b[17+8:], 8) }),
		"trailing garbage":    append(slices.Clone(good), 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12),
	}
	for name, data := range cases {
		if _, _, err := readAll(t, data, Limits{}); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s: err %v, want ErrCorrupt", name, err)
		}
	}
}

// The LZMA decoder reports a stream shorter than its 5-byte preamble with a
// bare io.EOF, which must not end iteration as if the file ended there.
func TestShortStream(t *testing.T) {
	short := []byte{
		0xed, 0xd1, 0xed, 0xd1, 4, 0, 0, 0, 9, 0, 0, 0, // marker, usz 4, csz 9
		4, 0, 0, 0, 0x5d, 0, 0, 1, 0, // inner size, properties, dictionary 64 KiB, no stream
	}
	data := append(fwrtest.File(fwrtest.Header{Version: 1}, []byte("good")), short...)
	data = append(data, fwrtest.Block([]byte("after"))...)
	_, blocks, err := readAll(t, data, Limits{})
	if !errors.Is(err, ErrCorrupt) || len(blocks) != 1 {
		t.Errorf("%d blocks, err %v", len(blocks), err)
	}
}

func TestLimits(t *testing.T) {
	kib := bytes.Repeat([]byte{7}, 1024)
	three := fwrtest.File(fwrtest.Header{Version: 1}, kib, kib, kib)

	_, blocks, err := readAll(t, three, Limits{MaxBlock: 1000})
	if !errors.Is(err, ErrLimit) || len(blocks) != 0 {
		t.Errorf("block limit: %d blocks, err %v", len(blocks), err)
	}
	_, blocks, err = readAll(t, three, Limits{MaxTotal: 2500})
	if !errors.Is(err, ErrLimit) || len(blocks) != 2 {
		t.Errorf("total limit: %d blocks, err %v", len(blocks), err)
	}
	if _, _, err = readAll(t, fwrtest.File(fwrtest.Header{Extra: make([]byte, 200)}, kib), Limits{MaxHeader: 100}); !errors.Is(err, ErrLimit) {
		t.Errorf("header limit: err %v", err)
	}
	// Multiplayer headers on workshop maps embed images.
	h, blocks, err := readAll(t, fwrtest.File(fwrtest.Header{Extra: make([]byte, 300<<10)}, kib), Limits{})
	if err != nil || h.FirstBlock != 17+300<<10 || len(blocks) != 1 {
		t.Errorf("300 KiB header: first block %d, %d blocks, err %v", h.FirstBlock, len(blocks), err)
	}
	// A short file declaring the largest header allocates what it holds.
	short := slices.Clone(three[:64])
	binary.LittleEndian.PutUint32(short[13:], 4<<20)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, _, err = readAll(t, short, Limits{})
	runtime.ReadMemStats(&after)
	if alloc := after.TotalAlloc - before.TotalAlloc; !errors.Is(err, ErrTruncated) || alloc > 1<<20 {
		t.Errorf("short header: err %v, %d bytes allocated", err, alloc)
	}

	huge := slices.Clone(three)
	copy(huge[17+4:], []byte{0xff, 0xff, 0xff, 0xff}) // declared decompressed size 4 GiB
	if _, _, err = readAll(t, huge, Limits{}); !errors.Is(err, ErrLimit) {
		t.Errorf("declared size: err %v", err)
	}
	hugeC := slices.Clone(three)
	copy(hugeC[17+8:], []byte{0xff, 0xff, 0xff, 0x7f}) // declared compressed size 2 GiB
	if _, _, err = readAll(t, hugeC, Limits{}); !errors.Is(err, ErrLimit) {
		t.Errorf("declared compressed size: err %v", err)
	}
	// 1024 declared bytes allow 9 + 1024 + 64 + 64 = 1161 compressed bytes.
	for csz, want := range map[uint32]error{1161: ErrTruncated, 1162: ErrLimit} {
		loose := slices.Clone(three)
		binary.LittleEndian.PutUint32(loose[17+8:], csz)
		if _, _, err = readAll(t, loose, Limits{}); !errors.Is(err, want) {
			t.Errorf("compressed size %d for 1024 bytes: err %v, want %v", csz, err, want)
		}
	}
	dict := slices.Clone(three)
	copy(dict[17+12+5:], []byte{0, 0, 0, 0x80}) // LZMA dictionary 2 GiB
	if _, _, err = readAll(t, dict, Limits{}); !errors.Is(err, ErrCorrupt) {
		t.Errorf("dictionary size: err %v", err)
	}
}

func TestNotAReplay(t *testing.T) {
	rnd := make([]byte, 4096)
	if _, err := rand.Read(rnd); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"empty": nil, "random": rnd, "text": []byte("data = {}\r\n")} {
		_, blocks, err := readAll(t, data, Limits{})
		if err == nil || len(blocks) != 0 {
			t.Errorf("%s: %d blocks, err %v", name, len(blocks), err)
		}
	}
}

func TestLiteralProperties(t *testing.T) {
	p := []byte{1, 0, 0, 0, 0xe0, 0, 0, 1, 0} // size 1; lc 8, lp 4; dictionary 64 KiB; no stream
	if _, err := decompress(p, 1, 1<<20); err == nil || !strings.Contains(err.Error(), "lc+lp") {
		t.Errorf("err %v", err)
	}
}
