package filename

type SBox struct {
	Forward [256]byte
	Inverse [256]byte
}

func GenerateSBox(seed []byte) *SBox {
	prng := newXorShiftPRNG(seed)

	box := [256]byte{}
	for i := range box {
		box[i] = byte(i)
	}

	for i := 255; i > 0; i-- {
		j := prng.next() % uint64(i+1)
		box[i], box[j] = box[j], box[i]
	}

	inverse := [256]byte{}
	for i := range box {
		inverse[box[i]] = byte(i)
	}

	return &SBox{
		Forward: box,
		Inverse: inverse,
	}
}

type xorShiftPRNG struct {
	state uint64
}

func newXorShiftPRNG(seed []byte) *xorShiftPRNG {
	var s uint64
	for i, b := range seed {
		s += uint64(b) << ((i % 8) * 8)
	}
	if s == 0 {
		s = 0x0F0F0F0F0F0F0F0F
	}
	return &xorShiftPRNG{state: s}
}

func (x *xorShiftPRNG) next() uint64 {
	x.state ^= x.state >> 12
	x.state ^= x.state << 25
	x.state ^= x.state >> 27
	return x.state * 0x2545F4914F6CDD1D
}
