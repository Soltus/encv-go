package reader

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
)

type fragmentRangeIndex struct {
	fragments []types.Fragment
	ends      []uint64
	totalSize int64
}

func newFragmentRangeIndex(fragments []types.Fragment) *fragmentRangeIndex {
	copied := append([]types.Fragment(nil), fragments...)
	sort.Slice(copied, func(i, j int) bool {
		return copied[i].GlobalStartOffset < copied[j].GlobalStartOffset
	})

	ends := make([]uint64, len(copied))
	var total uint64
	for i, frag := range copied {
		end := frag.GlobalStartOffset + frag.Length
		ends[i] = end
		if end > total {
			total = end
		}
	}

	return &fragmentRangeIndex{
		fragments: copied,
		ends:      ends,
		totalSize: int64(total),
	}
}

func (idx *fragmentRangeIndex) find(offset int64) (int, uint64, bool) {
	if idx == nil || len(idx.fragments) == 0 || offset < 0 {
		return -1, 0, false
	}
	if offset == idx.totalSize {
		return len(idx.fragments), 0, true
	}

	target := uint64(offset)
	pos := sort.Search(len(idx.ends), func(i int) bool {
		return idx.ends[i] > target
	})
	if pos >= len(idx.fragments) {
		return -1, 0, false
	}

	frag := idx.fragments[pos]
	if target < frag.GlobalStartOffset {
		return -1, 0, false
	}
	return pos, target - frag.GlobalStartOffset, true
}

func (idx *fragmentRangeIndex) total() int64 {
	if idx == nil {
		return 0
	}
	return idx.totalSize
}

func buildCTRStreamForBlock(block cipher.Block, baseIV []byte, absoluteOffset uint64) (cipher.Stream, error) {
	iv, err := crypto.DeriveCTRIVForOffset_v2(baseIV, absoluteOffset)
	if err != nil {
		return nil, err
	}

	stream := cipher.NewCTR(block, iv)
	if rem := absoluteOffset % uint64(aes.BlockSize); rem > 0 {
		var scratch [aes.BlockSize]byte
		stream.XORKeyStream(scratch[:rem], scratch[:rem])
	}
	return stream, nil
}

func buildCTRStreamAtOffset(key, baseIV []byte, absoluteOffset uint64) (cipher.Stream, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create aes cipher: %w", err)
	}
	return buildCTRStreamForBlock(block, baseIV, absoluteOffset)
}

func discardReaderBytes(r io.Reader, n uint64, pool *sync.Pool) error {
	if n == 0 {
		return nil
	}

	var buf []byte
	if pool != nil {
		if pooled, ok := pool.Get().([]byte); ok && cap(pooled) > 0 {
			buf = pooled[:cap(pooled)]
			defer pool.Put(buf)
		}
	}
	if len(buf) == 0 {
		buf = make([]byte, 32*1024)
	}

	remaining := n
	for remaining > 0 {
		chunk := uint64(len(buf))
		if remaining < chunk {
			chunk = remaining
		}
		readN, err := io.ReadFull(r, buf[:chunk])
		if err != nil {
			return fmt.Errorf("failed to discard %d bytes: %w", n, err)
		}
		remaining -= uint64(readN)
	}
	return nil
}
