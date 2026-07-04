package reader

import (
	"bytes"
	"fmt"
	"io"
	mathrand "math/rand"
	"testing"
)

func TestSequentialSeekReadConsistency(t *testing.T) {
	assertSeekReadConsistency(t, false)
}

func TestVirtualSeekReadConsistency(t *testing.T) {
	assertSeekReadConsistency(t, true)
}

func assertSeekReadConsistency(t *testing.T, useVirtual bool) {
	t.Helper()

	fragmentCounts := []int{1, 10, 100, 500}
	for _, fragCount := range fragmentCounts {
		name := "sequential"
		if useVirtual {
			name = "virtual"
		}
		t.Run(fmt.Sprintf("%s/frags=%d", name, fragCount), func(t *testing.T) {
			dataSize := int64(1 * 1024 * 1024)
			fixture := createContainerFixture(t, dataSize, fragCount)
			factoryForExpected, expectedErr := NewDecryptReaderFactory(fixture.ContainerPath, fixture.Password)
			if expectedErr != nil {
				t.Fatalf("create expected factory: %v", expectedErr)
			}
			expectedReader, expectedErr := factoryForExpected.NewDecryptReader()
			if expectedErr != nil {
				t.Fatalf("create expected reader: %v", expectedErr)
			}
			expectedData, expectedErr := io.ReadAll(expectedReader)
			_ = expectedReader.Close()
			_ = factoryForExpected.Close()
			if expectedErr != nil {
				t.Fatalf("read expected data: %v", expectedErr)
			}
			dataSize = int64(len(expectedData))

			var (
				reader DecryptReader
				err    error
				closeF func()
			)

			if useVirtual {
				cr, openErr := NewEncryptedContainerReaderFromFile(fixture.ContainerPath)
				if openErr != nil {
					t.Fatalf("open container reader: %v", openErr)
				}
				reader, err = NewVirtualSeekableDecryptReader(cr, fixture.Password)
				closeF = func() {
					_ = reader.Close()
					_ = cr.Close()
				}
			} else {
				factory, factoryErr := NewDecryptReaderFactory(fixture.ContainerPath, fixture.Password)
				if factoryErr != nil {
					t.Fatalf("create factory: %v", factoryErr)
				}
				reader, err = factory.NewDecryptReader()
				closeF = func() {
					_ = reader.Close()
					_ = factory.Close()
				}
			}
			if err != nil {
				t.Fatalf("create decrypt reader: %v", err)
			}
			defer closeF()

			seeker, ok := reader.(io.Seeker)
			if !ok {
				t.Fatalf("reader does not implement io.Seeker")
			}

			rng := mathrand.New(mathrand.NewSource(int64(1000 + fragCount)))
			for i := 0; i < 64; i++ {
				offset := rng.Int63n(dataSize)
				if _, err := seeker.Seek(offset, io.SeekStart); err != nil {
					t.Fatalf("seek failed at offset=%d: %v", offset, err)
				}

				chunkSize := int64(4096)
				if remaining := dataSize - offset; remaining < chunkSize {
					chunkSize = remaining
				}

				got := make([]byte, chunkSize)
				if _, err := io.ReadFull(reader, got); err != nil {
					t.Fatalf("read failed at offset=%d size=%d: %v", offset, chunkSize, err)
				}

				want := expectedData[offset : offset+chunkSize]
				if !bytes.Equal(got, want) {
					t.Fatalf("data mismatch at offset=%d size=%d", offset, chunkSize)
				}
			}
		})
	}
}
