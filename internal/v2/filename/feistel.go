package filename

func FeistelEncrypt(data []byte, sbox *SBox, roundKeys [][]byte) []byte {
	if len(data) == 0 {
		return data
	}
	n := len(data)
	padded := make([]byte, n)
	copy(padded, data)
	if n%2 != 0 {
		padded = append(padded, 0)
	}

	mid := len(padded) / 2
	left := padded[:mid]
	right := padded[mid:]

	for _, key := range roundKeys {
		f := roundFunc(right, key, sbox)
		for j := 0; j < len(left) && j < len(f); j++ {
			left[j] ^= f[j]
		}
		left, right = right, left
	}

	result := make([]byte, len(padded))
	copy(result, padded)
	return result
}

func FeistelDecrypt(data []byte, sbox *SBox, roundKeys [][]byte) []byte {
	if len(data) == 0 {
		return data
	}
	n := len(data)
	padded := make([]byte, n)
	copy(padded, data)
	if n%2 != 0 {
		padded = append(padded, 0)
	}

	mid := len(padded) / 2
	left := padded[:mid]
	right := padded[mid:]

	left, right = right, left

	for i := len(roundKeys) - 1; i >= 0; i-- {
		f := roundFunc(right, roundKeys[i], sbox)
		for j := 0; j < len(left) && j < len(f); j++ {
			left[j] ^= f[j]
		}
		if i > 0 {
			left, right = right, left
		}
	}

	result := make([]byte, len(padded))
	copy(result, padded)
	return result
}

func roundFunc(half, key []byte, sbox *SBox) []byte {
	out := make([]byte, len(half))
	minLen := len(half)
	if len(key) < minLen {
		minLen = len(key)
	}
	for i := 0; i < minLen; i++ {
		out[i] = sbox.Forward[(int(half[i])+int(key[i]))&0xFF]
	}
	for i := minLen; i < len(out); i++ {
		out[i] = sbox.Forward[half[i]]
	}
	return out
}
