package reader

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
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

// decodeFragmentNonce 解出分片自带的 nonce（base64）。
//
// v4 segment 栈给每段生成一个随机 nonce，适配成 fragment 时必须带过来
// （types.Fragment.Nonce），读取端才能按段重置 keystream；
// 为空表示"沿用 KVI 的 iv"，即旧容器的单一 keystream 行为。
// 解不出来就当作没有（返回 nil，走旧路径），不把一次 base64 失败变成致命错误。
func decodeFragmentNonce(nonceBase64 string) []byte {
	if nonceBase64 == "" {
		return nil
	}
	nonce, err := base64.StdEncoding.DecodeString(nonceBase64)
	if err != nil {
		return nil
	}
	return nonce
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
