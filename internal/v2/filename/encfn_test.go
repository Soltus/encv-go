package filename

import (
	"bytes"
	"strings"
	"testing"
)

func compactCfg() FNConfig {
	return FNConfig{Password: []byte("test-password"), Salt: []byte("salt"), Charsets: []FNCharset{}, Deconfuse: true, Rounds: 6, Structured: false}
}

func structuredCfg() FNConfig {
	return FNConfig{Password: []byte("test-password"), Salt: []byte("salt"), Charsets: []FNCharset{}, Deconfuse: true, Rounds: 6, Structured: true}
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"english filename", "video.mp4"},
		{"long mixed chinese english", "2024年度财务报表_Q3_final_version.pdf"},
		{"emoji", "照片🎉2024.jpg"},
		{"chinese", "中文文件名测试.txt"},
		{"rare hanzi", "龘靁齉爨麤毊.doc"},
		{"spaces only", "   "},
		{"special chars", "file-with_spaces.and-dots.tar.gz"},
		{"control chars", "\x00\x01\x02"},
		{"single byte", "a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for modeName, cfg := range map[string]FNConfig{
				"compact":    compactCfg(),
				"structured": structuredCfg(),
			} {
				t.Run(modeName, func(t *testing.T) {
					plaintext := []byte(tt.input)
					encoded, err := cfg.Encode(plaintext)
					if err != nil {
						t.Fatalf("Encode failed: %v", err)
					}
					if len(encoded) == 0 {
						t.Fatal("encoded output is empty")
					}

					decoded, err := cfg.Decode(encoded)
					if err != nil {
						t.Fatalf("Decode failed: %v", err)
					}
					if !bytes.Equal(plaintext, decoded) {
						t.Errorf("roundtrip mismatch\n  input:  %q (%x)\n  output: %q (%x)", plaintext, plaintext, decoded, decoded)
					}
				})
			}
		})
	}

	t.Run("long_input_500_bytes", func(t *testing.T) {
		longInput := make([]byte, 500)
		for i := range longInput {
			longInput[i] = byte(i % 256)
		}
		for modeName, cfg := range map[string]FNConfig{
			"compact":    compactCfg(),
			"structured": structuredCfg(),
		} {
			t.Run(modeName, func(t *testing.T) {
				encoded, err := cfg.Encode(longInput)
				if err != nil {
					t.Fatalf("Encode failed: %v", err)
				}
				decoded, err := cfg.Decode(encoded)
				if err != nil {
					t.Fatalf("Decode failed: %v", err)
				}
				if !bytes.Equal(longInput, decoded) {
					t.Errorf("roundtrip mismatch for 500-byte input, got %d bytes want %d", len(decoded), len(longInput))
				}
			})
		}
	})
}

func TestPasswordSensitivity(t *testing.T) {
	plaintext := []byte("sensitive-data.txt")

	passwords := [][]byte{
		[]byte("password-A"),
		[]byte("password-B"),
		[]byte(""),
		[]byte("x"),
	}

	var outputs []string
	for i, pwd := range passwords {
		cfg := FNConfig{Password: pwd, Salt: []byte("salt"), Rounds: 6}
		encoded, err := cfg.Encode(plaintext)
		if err != nil {
			t.Fatalf("password[%d] Encode failed: %v", i, err)
		}
		outputs = append(outputs, encoded)

		for j, other := range outputs[:len(outputs)-1] {
			if encoded == other {
				t.Errorf("passwords[%d]=%q and passwords[%d]=%q produced identical output: %s", i, pwd, j, passwords[j], encoded)
			}
		}
	}
}

func TestDeterminism(t *testing.T) {
	plaintext := []byte("deterministic-test.dat")

	cfg := FNConfig{Password: []byte("fixed-password"), Salt: []byte("fixed-salt"), Rounds: 8, Deconfuse: true}

	var firstOutput string
	for i := 0; i < 5; i++ {
		encoded, err := cfg.Encode(plaintext)
		if err != nil {
			t.Fatalf("iteration %d Encode failed: %v", i, err)
		}
		if i == 0 {
			firstOutput = encoded
		} else if encoded != firstOutput {
			t.Errorf("iteration %d produced different output than iteration 0", i)
		}
	}

	for i := 0; i < 5; i++ {
		decoded, err := cfg.Decode(firstOutput)
		if err != nil {
			t.Fatalf("iteration %d Decode failed: %v", i, err)
		}
		if !bytes.Equal(plaintext, decoded) {
			t.Errorf("iteration %d decode mismatch", i)
		}
	}
}

func TestBuildCharsetTable(t *testing.T) {
	tests := []struct {
		name            string
		charsets        []FNCharset
		deconfuse       bool
		wantSize        int
		wantErr         bool
		wantContains    []rune
		wantNotContains []rune
	}{
		{
			name:      "alnum_only_no_deconfuse",
			charsets:  []FNCharset{},
			deconfuse: false,
			wantSize:  62,
		},
		{
			name:      "alnum_only_deconfuse",
			charsets:  []FNCharset{},
			deconfuse: true,
			wantSize:  56,
		},
		{
			name:            "hanzi_rare_deconfuse",
			charsets:        []FNCharset{FNHanziRare},
			deconfuse:       true,
			wantSize:        -1,
			wantNotContains: []rune{'0', 'O', 'o', '1', 'l', 'I'},
		},
		{
			name:         "symbols_basic",
			charsets:     []FNCharset{FNSymbolsBasic},
			deconfuse:    false,
			wantContains: []rune{'_', '-', '.', '~'},
		},
		{
			name:         "emoji_charset",
			charsets:     []FNCharset{FNEmoji},
			deconfuse:    false,
			wantContains: []rune(EmojiChars)[:5],
		},
		{
			name:     "unknown_charset",
			charsets: []FNCharset{FNCharset("unknown")},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table, err := BuildCharsetTable(tt.charsets, tt.deconfuse)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantSize > 0 && len(table) != tt.wantSize {
				t.Errorf("table size = %d, want %d", len(table), tt.wantSize)
			}
			if tt.wantSize < 0 && len(table) <= 56 {
				t.Errorf("table size with hanzi_rare+deconfuse should be > 56, got %d", len(table))
			}
			for _, r := range tt.wantContains {
				found := false
				for _, tr := range table {
					if tr == r {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("table missing expected rune %c (%U)", r, r)
				}
			}
			for _, r := range tt.wantNotContains {
				for _, tr := range table {
					if tr == r {
						t.Errorf("table contains unexpected rune %c (%U) that should be deconfused", r, r)
					}
				}
			}
		})
	}

	t.Run("union_size_multiple_charsets", func(t *testing.T) {
		table, err := BuildCharsetTable([]FNCharset{FNSymbolsBasic, FNSymbolsExt}, false)
		if err != nil {
			t.Fatalf("BuildCharsetTable failed: %v", err)
		}
		alnumLen := len(AlnumChars)
		symbolsBasicLen := len(SymbolsBasicChars)
		symbolsExtLen := len(SymbolsExtChars)
		maxPossible := alnumLen + symbolsBasicLen + symbolsExtLen
		if len(table) > maxPossible {
			t.Errorf("table size %d exceeds max possible %d", len(table), maxPossible)
		}
		if len(table) < alnumLen+symbolsBasicLen {
			t.Errorf("table size %d should be at least alnum+basic=%d", len(table), alnumLen+symbolsBasicLen)
		}
	})
}

func TestDeconfuseRemoval(t *testing.T) {
	confusableRunes := []rune{'0', 'O', 'o', '1', 'l', 'I'}

	t.Run("deconfuse_true_removes_confusables", func(t *testing.T) {
		table, err := BuildCharsetTable([]FNCharset{}, true)
		if err != nil {
			t.Fatalf("BuildCharsetTable failed: %v", err)
		}
		for _, r := range confusableRunes {
			for _, tr := range table {
				if tr == r {
					t.Errorf("deconfuse=true table still contains confusable rune %c", r)
				}
			}
		}
	})

	t.Run("deconfuse_false_keeps_confusables", func(t *testing.T) {
		table, err := BuildCharsetTable([]FNCharset{}, false)
		if err != nil {
			t.Fatalf("BuildCharsetTable failed: %v", err)
		}
		for _, r := range confusableRunes {
			found := false
			for _, tr := range table {
				if tr == r {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("deconfuse=false table missing expected rune %c", r)
			}
		}
	})
}

func TestStructuredFormat(t *testing.T) {
	cfg := structuredCfg()
	plaintext := []byte("hello-world-test.data")

	encoded, err := cfg.Encode(plaintext)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if !strings.HasPrefix(encoded, "S") {
		t.Errorf("structured output should start with 'S', got prefix: %q", encoded[:min(3, len(encoded))])
	}

	if !strings.ContainsRune(encoded, ':') {
		t.Error("structured output should contain ':' separators")
	}

	parts := strings.Split(encoded, ":")
	if len(parts) < 2 {
		t.Fatalf("structured output should have at least 2 colon-separated parts, got %d", len(parts))
	}

	bodyPart := parts[1]
	if len(bodyPart) == 0 {
		t.Error("structured body is empty")
	}

	checkPart := parts[len(parts)-1]
	if !strings.Contains(checkPart, ",") {
		t.Error("checksum part should contain ',' separating table CRC and data CRC")
	}

	t.Run("contains_original_length", func(t *testing.T) {
		commaParts := strings.Split(checkPart, ",")
		if len(commaParts) < 3 {
			t.Fatalf("expected at least 3 comma-separated parts in check section (tableCRC,dataCRC,originalLen), got %d", len(commaParts))
		}
		lenStr := commaParts[2]
		if lenStr == "" {
			t.Error("original length field is empty")
		}
	})

	t.Run("decode_roundtrip", func(t *testing.T) {
		decoded, err := cfg.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if !bytes.Equal(plaintext, decoded) {
			t.Errorf("decoded = %q, want %q", decoded, plaintext)
		}
	})
}

func TestDecodeErrors(t *testing.T) {
	cfg := structuredCfg()

	t.Run("empty_string_returns_error", func(t *testing.T) {
		_, err := cfg.Decode("")
		if err == nil {
			t.Error("expected error for empty input, got nil")
		}
		if err != ErrFNEmptyInput {
			t.Errorf("expected ErrFNEmptyInput, got: %v", err)
		}
	})

	t.Run("invalid_format_xyz", func(t *testing.T) {
		_, err := cfg.Decode("xyz")
		if err == nil {
			t.Error("expected error for invalid format 'xyz', got nil")
		}
	})

	t.Run("corrupted_structured_body", func(t *testing.T) {
		plaintext := []byte("test-corruption-check")
		encoded, err := cfg.Encode(plaintext)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}

		runes := []rune(encoded)
		if len(runes) > 10 {
			mid := len(runes)/2 - 2
			if mid >= 0 && mid < len(runes) {
				runes[mid] = 'X'
				corrupted := string(runes)
				_, decodeErr := cfg.Decode(corrupted)
				if decodeErr == nil {
					t.Error("expected error for corrupted body, got nil")
				} else if decodeErr != ErrFNChecksumMismatch && decodeErr != ErrFNInvalidFormat && decodeErr != ErrFNCorrupt && decodeErr != ErrFNCharsetMismatch {
					t.Logf("corruption returned error type: %T: %v", decodeErr, decodeErr)
				}
			}
		}
	})

	t.Run("compact_invalid_format", func(t *testing.T) {
		compactCfg := FNConfig{Password: []byte("pw"), Salt: []byte("s"), Rounds: 6}
		_, err := compactCfg.Decode("!!not-valid-output!!")
		if err == nil {
			t.Error("expected error for invalid compact format, got nil")
		}
	})
}

func TestDifferentCharsetsRoundtrip(t *testing.T) {
	testCases := []struct {
		name      string
		charsets  []FNCharset
		deconfuse bool
	}{
		{"alnum_only", []FNCharset{}, true},
		{"alnum_hanzi_rare", []FNCharset{FNHanziRare}, true},
		{"alnum_emoji", []FNCharset{FNEmoji}, true},
		{"all_extensions", []FNCharset{FNSymbolsBasic, FNSymbolsExt, FNHanziRare, FNEmoji}, true},
	}

	plaintext := []byte("charset-roundtrip-test-数据测试🔐")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := FNConfig{
				Password:  []byte("charset-test-pw"),
				Salt:      []byte("charset-salt"),
				Charsets:  tc.charsets,
				Deconfuse: tc.deconfuse,
				Rounds:    6,
			}

			table, err := BuildCharsetTable(tc.charsets, tc.deconfuse)
			if err != nil {
				t.Fatalf("BuildCharsetTable failed: %v", err)
			}
			if len(table) == 0 {
				t.Fatal("charset table is empty")
			}

			encoded, err := cfg.Encode(plaintext)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			decoded, err := cfg.Decode(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			if !bytes.Equal(plaintext, decoded) {
				t.Errorf("roundtrip mismatch with charsets %v\n  input:  %q\n  output: %q", tc.charsets, plaintext, decoded)
			}
		})
	}
}

func TestEncodeEmptyInput(t *testing.T) {
	cfg := compactCfg()
	_, err := cfg.Encode([]byte{})
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
	if err != ErrFNEmptyInput {
		t.Errorf("expected ErrFNEmptyInput, got: %v", err)
	}
}

func TestDecodeEmptyInput(t *testing.T) {
	cfg := compactCfg()
	_, err := cfg.Decode("")
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
	if err != ErrFNEmptyInput {
		t.Errorf("expected ErrFNEmptyInput, got: %v", err)
	}
}

func TestValidateDefaults(t *testing.T) {
	cfg := FNConfig{}
	cfg.Validate()
	if cfg.Rounds < 1 {
		t.Errorf("Validate should set default Rounds >= 1, got %d", cfg.Rounds)
	}
	if len(cfg.Charsets) == 0 {
		t.Error("Validate should set default Charsets to [FNAlnum]")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
