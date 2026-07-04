package filename

import (
	"fmt"
	"strings"
	"testing"
)

func TestEncodeToCharset_DecodeFromCharset_roundtrip(t *testing.T) {
	table, err := BuildCharsetTable([]FNCharset{FNAlnum}, false)
	if err != nil {
		t.Fatalf("BuildCharsetTable failed: %v", err)
	}

	tests := []struct {
		name string
		data []byte
	}{
		{"single byte", []byte{0x42}},
		{"two bytes", []byte{0x00, 0xFF}},
		{"ascii hello", []byte("hello")},
		{"all 256 bytes", func() []byte {
			b := make([]byte, 256)
			for i := range b {
				b[i] = byte(i)
			}
			return b
		}()},
		{"100 zero bytes", make([]byte, 100)},
		{"mixed high bytes", []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE}},
		{"single null", []byte{0x00}},
		{"single 0xFF", []byte{0xFF}},
		{"repeating pattern", []byte{0xAB, 0xCD, 0xAB, 0xCD, 0xAB}},
		{"sequential", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeToCharset(tt.data, table)
			if len(encoded) == 0 && len(tt.data) > 0 {
				t.Fatal("EncodeToCharset returned empty for non-empty input")
			}

			decoded, err := DecodeFromCharset(encoded, table)
			if err != nil {
				t.Fatalf("DecodeFromCharset failed: %v", err)
			}
			if len(decoded) != len(tt.data) {
				t.Fatalf("length mismatch: got %d want %d", len(decoded), len(tt.data))
			}
			for i := range tt.data {
				if decoded[i] != tt.data[i] {
					t.Errorf("byte[%d] mismatch: got 0x%02X want 0x%02X", i, decoded[i], tt.data[i])
					break
				}
			}
		})
	}
}

func TestEncodeToCharset_DecodeFromCharset_roundtrip_multi_charset(t *testing.T) {
	charsetCombos := [][]FNCharset{
		{},
		{FNSymbolsBasic},
		{FNSymbolsExt},
		{FNHanziRare},
		{FNEmoji},
		{FNSymbolsBasic, FNSymbolsExt},
		{FNAlnum, FNHanziRare, FNEmoji},
		{FNSymbolsBasic, FNHanziRare, FNEmoji, FNSymbolsExt},
	}

	input := []byte("multi_charset_roundtrip_data")

	for _, combo := range charsetCombos {
		name := ""
		for _, cs := range combo {
			name += string(cs) + "_"
		}
		t.Run(name, func(t *testing.T) {
			for _, deconfuse := range []bool{false, true} {
				dn := "nodeconfuse"
				if deconfuse {
					dn = "deconfuse"
				}
				t.Run(dn, func(t *testing.T) {
					table, err := BuildCharsetTable(combo, deconfuse)
					if err != nil {
						t.Fatalf("BuildCharsetTable failed: %v", err)
					}

					encoded := EncodeToCharset(input, table)
					decoded, err := DecodeFromCharset(encoded, table)
					if err != nil {
						t.Fatalf("DecodeFromCharset failed: %v", err)
					}
					if string(decoded) != string(input) {
						t.Errorf("roundtrip mismatch: got %q want %q", string(decoded), string(input))
					}
				})
			}
		})
	}
}

func TestDeconfuseRemovesConfusableChars(t *testing.T) {
	confusableRunes := []rune{'0', 'O', 'o', '1', 'l', 'I'}

	t.Run("alnum without deconfuse contains all confusables", func(t *testing.T) {
		table, err := BuildCharsetTable([]FNCharset{}, false)
		if err != nil {
			t.Fatalf("BuildCharsetTable failed: %v", err)
		}
		for _, cr := range confusableRunes {
			found := false
			for _, tr := range table {
				if tr == cr {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("non-deconfused alnum table should contain %q", cr)
			}
		}
	})

	t.Run("alnum with deconfuse removes all confusables", func(t *testing.T) {
		table, err := BuildCharsetTable([]FNCharset{}, true)
		if err != nil {
			t.Fatalf("BuildCharsetTable failed: %v", err)
		}
		for _, cr := range confusableRunes {
			for _, tr := range table {
				if tr == cr {
					t.Errorf("deconfused alnum table should NOT contain %q", cr)
				}
			}
		}
	})

	t.Run("hanzi_rare with deconfuse still removes alnum confusables", func(t *testing.T) {
		table, err := BuildCharsetTable([]FNCharset{FNHanziRare}, true)
		if err != nil {
			t.Fatalf("BuildCharsetTable failed: %v", err)
		}
		for _, cr := range confusableRunes {
			for _, tr := range table {
				if tr == cr {
					t.Errorf("deconfused hanzi_rare table should NOT contain %q", cr)
				}
			}
		}
	})

	t.Run("emoji with deconfuse still removes alnum confusables", func(t *testing.T) {
		table, err := BuildCharsetTable([]FNCharset{FNEmoji}, true)
		if err != nil {
			t.Fatalf("BuildCharsetTable failed: %v", err)
		}
		for _, cr := range confusableRunes {
			for _, tr := range table {
				if tr == cr {
					t.Errorf("deconfused emoji table should NOT contain %q", cr)
				}
			}
		}
	})

	t.Run("deconfuse size reduction is exactly 6 for pure alnum", func(t *testing.T) {
		tableNoDeconfuse, _ := BuildCharsetTable([]FNCharset{}, false)
		tableDeconfuse, _ := BuildCharsetTable([]FNCharset{}, true)
		reduction := len(tableNoDeconfuse) - len(tableDeconfuse)
		if reduction != 6 {
			t.Errorf("deconfuse should remove exactly 6 chars from alnum, got reduction=%d (no_deconfuse=%d, deconfuse=%d)",
				reduction, len(tableNoDeconfuse), len(tableDeconfuse))
		}
	})

	t.Run("deconfused table preserves non-confusable alnum chars", func(t *testing.T) {
		table, _ := BuildCharsetTable([]FNCharset{}, true)
		sampleNonConfusable := "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
		for _, expected := range sampleNonConfusable {
			found := false
			for _, tr := range table {
				if tr == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("deconfused table should still contain non-confusable char %q", expected)
			}
		}
	})
}

func TestEncodeToCharset_empty_input(t *testing.T) {
	table, _ := BuildCharsetTable([]FNCharset{}, false)

	result := EncodeToCharset([]byte{}, table)
	if result != "" {
		t.Errorf("empty input should return empty string, got %q", result)
	}
}

func TestDecodeFromCharset_empty_input(t *testing.T) {
	table, _ := BuildCharsetTable([]FNCharset{}, false)

	result, err := DecodeFromCharset("", table)
	if err != nil {
		t.Fatalf("empty string decode should return nil error, got %v", err)
	}
	if result != nil {
		t.Errorf("empty string decode should return nil data, got %v", result)
	}
}

func TestDecodeFromCharset_bad_char_width(t *testing.T) {
	table, _ := BuildCharsetTable([]FNCharset{}, false)

	charWidth := charsetCharWidth(uint64(len(table)))
	badInput := strings.Repeat("A", charWidth+1)

	_, err := DecodeFromCharset(badInput, table)
	if err != ErrFNInvalidFormat {
		t.Errorf("mismatched length should return ErrFNInvalidFormat, got %v (%T)", err, err)
	}
}

func TestDecodeFromCharset_unknown_char_in_table(t *testing.T) {
	table, _ := BuildCharsetTable([]FNCharset{}, true)

	encoded := EncodeToCharset([]byte{0x42}, table)
	if len(encoded) < 2 {
		t.Skip("encoded too short to tamper")
	}

	runes := []rune(encoded)
	runes[0] = '\uffff'
	tampered := string(runes)

	_, err := DecodeFromCharset(tampered, table)
	if err != ErrFNCharsetMismatch {
		t.Errorf("unknown char in encoded string should return ErrFNCharsetMismatch, got %v (%T)", err, err)
	}
}

func TestCharsetCharWidth_values(t *testing.T) {
	tests := []struct {
		base     uint64
		expected int
	}{
		{56, 2},
		{57, 2},
		{62, 2},
		{63, 2},
		{255, 2},
		{256, 1},
		{257, 1},
		{256 * 256, 1},
		{255*255 + 1, 1},
		{16, 2},
		{10, 3},
		{2, 8},
	}

	for _, tt := range tests {
		t.Run(strings.Map(func(r rune) rune {
			if r == '_' {
				return 'B'
			}
			return r
		}, fmt.Sprintf("base_%d", tt.base)), func(t *testing.T) {
			got := charsetCharWidth(tt.base)
			if got != tt.expected {
				t.Errorf("charsetCharWidth(%d) = %d, want %d", tt.base, got, tt.expected)
			}
		})
	}
}

func TestBuildCharsetTable_deduplication(t *testing.T) {
	table, _ := BuildCharsetTable([]FNCharset{FNAlnum, FNAlnum, FNAlnum}, false)

	if len(table) != 62 {
		t.Errorf("duplicate FNAlnum should not increase size: got %d want 62", len(table))
	}

	countA := 0
	for _, r := range table {
		if r == 'A' {
			countA++
		}
	}
	if countA != 1 {
		t.Errorf("'A' appears %d times, should appear exactly once", countA)
	}
}

func TestBuildCharsetTable_all_charset_combinations_valid(t *testing.T) {
	allCharsets := []FNCharset{
		FNAlnum,
		FNSymbolsBasic,
		FNSymbolsExt,
		FNHanziRare,
		FNEmoji,
	}

	for _, cs := range allCharsets {
		t.Run(string(cs), func(t *testing.T) {
			table, err := BuildCharsetTable([]FNCharset{cs}, true)
			if err != nil {
				t.Fatalf("BuildCharsetTable(%s) failed: %v", cs, err)
			}
			if len(table) == 0 {
				t.Error("table should not be empty")
			}
		})
	}
}

func TestContainsRune(t *testing.T) {
	tests := []struct {
		slice []rune
		r     rune
		want  bool
	}{
		{[]rune("abc"), 'a', true},
		{[]rune("abc"), 'z', false},
		{[]rune(""), 'a', false},
		{[]rune("a"), 'a', true},
		{nil, 'a', false},
		{[]rune("龘"), '龘', true},
		{[]rune("龘"), '靁', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			got := containsRune(tt.slice, tt.r)
			if got != tt.want {
				t.Errorf("containsRune(%q, %q) = %v, want %v", string(tt.slice), tt.r, got, tt.want)
			}
		})
	}
}
