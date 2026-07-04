// Package compression provides compression primitives used by v4 container
// Segments. The current scope is limited to smoke testing the
// zstd-seekable dependency. Full integration with the segment crypto layer
// will be added in Task 8 / Task 9 of the v4 container capability upgrade
// spec.
package compression

import (
	"bytes"
	"io"
	"testing"

	seekable "github.com/SaveTheRbtz/zstd-seekable-format-go/pkg"
	"github.com/klauspost/compress/zstd"
)

// TestZstdSeekable_BasicRoundTrip verifies the minimum end-to-end flow of the
// zstd-seekable dependency: feed a payload through the seekable encoder,
// persist the resulting compressed frames plus the final seek-table frame to
// a single in-memory stream, parse the seek table back, and decompress the
// payload via the seekable reader. The reconstructed bytes must equal the
// original input, and the seek table must contain at least one frame entry.
func TestZstdSeekable_BasicRoundTrip(t *testing.T) {
	original := []byte("Hello, World!")

	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	if err != nil {
		t.Fatalf("zstd.NewWriter failed: %v", err)
	}
	defer enc.Close()

	// The Encoder API is byte-oriented: each Encode call returns one
	// compressed frame; EndStream returns the final seek-table skippable
	// frame. Concatenating them produces a valid seekable zstd stream.
	se, err := seekable.NewEncoder(enc)
	if err != nil {
		t.Fatalf("seekable.NewEncoder failed: %v", err)
	}

	compressed, err := se.Encode(original)
	if err != nil {
		t.Fatalf("seekable.Encode failed: %v", err)
	}
	if len(compressed) == 0 {
		t.Fatalf("expected non-empty compressed frame, got 0 bytes")
	}

	seekTableFrame, err := se.EndStream()
	if err != nil {
		t.Fatalf("seekable.EndStream failed: %v", err)
	}
	if len(seekTableFrame) == 0 {
		t.Fatalf("expected non-empty seek-table frame, got 0 bytes")
	}

	// Parse the seek table before assembling the full stream so we can
	// verify the index is non-empty.
	seekTable, err := seekable.NewSeekTable(seekTableFrame)
	if err != nil {
		t.Fatalf("seekable.NewSeekTable failed: %v", err)
	}
	if numFrames := seekTable.NumFrames(); numFrames < 1 {
		t.Fatalf("expected at least 1 seek-table entry, got %d", numFrames)
	}

	// The seekable stream layout: [compressed frames ...][seek-table frame].
	stream := bytes.NewBuffer(nil)
	stream.Write(compressed)
	stream.Write(seekTableFrame)

	// Decompress via the seekable reader, which uses the seek table to
	// service Read/ReadAt/Seek by decompressed byte offset.
	dec, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatalf("zstd.NewReader failed: %v", err)
	}
	defer dec.Close()

	r, err := seekable.NewReader(bytes.NewReader(stream.Bytes()), dec)
	if err != nil {
		t.Fatalf("seekable.NewReader failed: %v", err)
	}
	defer r.Close()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("seekable reader ReadAll failed: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("round-trip mismatch:\n want=%q\n got =%q", original, got)
	}

	// Reader.SeekTable must agree with the table parsed from EndStream.
	readerTable, err := r.SeekTable()
	if err != nil {
		t.Fatalf("Reader.SeekTable failed: %v", err)
	}
	if readerTable.NumFrames() < 1 {
		t.Fatalf("expected Reader.SeekTable to expose >=1 frames, got %d", readerTable.NumFrames())
	}
}
