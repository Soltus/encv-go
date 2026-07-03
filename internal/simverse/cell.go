package simverse

import (
	"math/rand"
)

type FractalLayer int

const (
	FractalLayerWorld FractalLayer = 0
	FractalLayerNPC   FractalLayer = 1
	FractalLayerCell  FractalLayer = 2
)

type Cell struct {
	ID             uint64
	HostNPCID      uint64
	CellType       CellType
	State          CellState
	Activity       uint16
	Connectivity   uint16
	Plasticity     uint8
	Age            uint16
	MaxAge         uint16
	IsAlive        bool
	LastUpdateTick uint32
	RegionID       uint16
	ClusterID      uint16
	Energy         uint16
	MaxEnergy      uint16
	FireCount      uint32
	WeightSum      uint32
}

type CellType uint8

const (
	CellTypePyramidal    CellType = 0
	CellTypeInterneuron  CellType = 1
	CellTypeGlial        CellType = 2
	CellTypeStem         CellType = 3
	CellTypeMemory       CellType = 4
	CellTypeEmotion      CellType = 5
	CellTypeDecision     CellType = 6
	CellTypeMax          CellType = 7
)

var CellTypeNames = map[CellType]string{
	CellTypePyramidal:   "锥体细胞",
	CellTypeInterneuron: "中间神经元",
	CellTypeGlial:       "胶质细胞",
	CellTypeStem:        "干细胞",
	CellTypeMemory:      "记忆细胞",
	CellTypeEmotion:     "情绪细胞",
	CellTypeDecision:    "决策细胞",
}

type CellState uint8

const (
	CellStateResting    CellState = 0
	CellStateActive     CellState = 1
	CellStateFiring     CellState = 2
	CellStateRefractory CellState = 3
	CellStatePlastic    CellState = 4
	CellStateDying      CellState = 5
)

type BrainRegion uint16

const (
	BrainRegionFrontal    BrainRegion = 0
	BrainRegionParietal   BrainRegion = 1
	BrainRegionTemporal   BrainRegion = 2
	BrainRegionOccipital  BrainRegion = 3
	BrainRegionHippocampus BrainRegion = 4
	BrainRegionAmygdala   BrainRegion = 5
	BrainRegionCerebellum BrainRegion = 6
	BrainRegionBrainstem  BrainRegion = 7
	BrainRegionMax        BrainRegion = 8
)

var BrainRegionNames = map[BrainRegion]string{
	BrainRegionFrontal:     "前额叶",
	BrainRegionParietal:    "顶叶",
	BrainRegionTemporal:    "颞叶",
	BrainRegionOccipital:   "枕叶",
	BrainRegionHippocampus: "海马体",
	BrainRegionAmygdala:    "杏仁核",
	BrainRegionCerebellum:  "小脑",
	BrainRegionBrainstem:   "脑干",
}

type BrainStats struct {
	TotalCells     uint64
	ActiveCells    uint32
	Connections    uint64
	ActivityLevel  float64
	Plasticity     float64
	RegionStats    [BrainRegionMax]RegionStats
}

type RegionStats struct {
	CellCount   uint32
	ActivityAvg uint16
	WeightAvg   uint16
}

type Brain struct {
	HostNPCID    uint64
	Stats        BrainStats
	CoreClusters []uint16
}

func NewBrain(hostNPCID uint64) *Brain {
	b := &Brain{
		HostNPCID: hostNPCID,
	}
	b.Stats.TotalCells = 86_000_000_000
	return b
}

func (b *Brain) CatchUp(currentTick uint32, rng *rand.Rand, npc *NPCV3) {
}

func (b *Brain) GetActiveCells(region BrainRegion, count int, npcAge uint16, rng *rand.Rand) []Cell {
	result := make([]Cell, 0, count)
	regionStat := b.Stats.RegionStats[region]
	if regionStat.CellCount == 0 {
		return result
	}

	baseID := uint64(b.HostNPCID)<<32 | uint64(region)<<16
	for i := 0; i < count; i++ {
		cellIdx := rng.Intn(int(regionStat.CellCount))
		cell := Cell{
			ID:             baseID + uint64(cellIdx),
			HostNPCID:      b.HostNPCID,
			CellType:       CellType(rng.Intn(int(CellTypeMax))),
			State:          CellState(rng.Intn(3)),
			Activity:       uint16(rng.Intn(int(regionStat.ActivityAvg)) + 50),
			Connectivity:   uint16(rng.Intn(1000) + 100),
			Plasticity:     uint8(rng.Intn(100)),
			Age:            uint16(rng.Intn(int(npcAge*100)) + 10),
			MaxAge:         uint16(rng.Intn(5000) + 1000),
			IsAlive:        true,
			LastUpdateTick: 0,
			RegionID:       uint16(region),
			ClusterID:      uint16(rng.Intn(100)),
			Energy:         uint16(rng.Intn(500) + 300),
			MaxEnergy:      1000,
		}
		result = append(result, cell)
	}
	return result
}

func (b *Brain) Think(thoughtType string, intensity float64, rng *rand.Rand) {
	switch thoughtType {
	case "memory_recall":
		b.activateRegion(BrainRegionHippocampus, intensity, rng)
	case "emotion":
		b.activateRegion(BrainRegionAmygdala, intensity, rng)
	case "decision":
		b.activateRegion(BrainRegionFrontal, intensity, rng)
	case "perception":
		b.activateRegion(BrainRegionOccipital, intensity*0.7, rng)
		b.activateRegion(BrainRegionTemporal, intensity*0.5, rng)
	}
}

func (b *Brain) activateRegion(region BrainRegion, intensity float64, rng *rand.Rand) {
	stat := &b.Stats.RegionStats[region]
	delta := uint16(intensity * 500 * rng.Float64())
	if stat.ActivityAvg+delta > 1000 {
		stat.ActivityAvg = 1000
	} else {
		stat.ActivityAvg += delta
	}
}

func (c *Cell) MarshalTo(buf []byte) int {
	off := 0
	binaryPutUint64(buf[off:], c.ID)
	off += 8
	binaryPutUint64(buf[off:], c.HostNPCID)
	off += 8
	buf[off] = byte(c.CellType)
	off++
	buf[off] = byte(c.State)
	off++
	binaryPutUint16(buf[off:], c.Activity)
	off += 2
	binaryPutUint16(buf[off:], c.Connectivity)
	off += 2
	buf[off] = c.Plasticity
	off++
	binaryPutUint16(buf[off:], c.Age)
	off += 2
	binaryPutUint16(buf[off:], c.MaxAge)
	off += 2
	if c.IsAlive {
		buf[off] = 1
	} else {
		buf[off] = 0
	}
	off++
	binaryPutUint32(buf[off:], uint32(c.LastUpdateTick))
	off += 4
	binaryPutUint16(buf[off:], c.RegionID)
	off += 2
	binaryPutUint16(buf[off:], c.ClusterID)
	off += 2
	binaryPutUint16(buf[off:], c.Energy)
	off += 2
	binaryPutUint16(buf[off:], c.MaxEnergy)
	off += 2
	binaryPutUint32(buf[off:], c.FireCount)
	off += 4
	binaryPutUint32(buf[off:], c.WeightSum)
	off += 4
	return off
}

func (c *Cell) Unmarshal(buf []byte) bool {
	if len(buf) < 44 {
		return false
	}
	off := 0
	c.ID = binaryGetUint64(buf[off:])
	off += 8
	c.HostNPCID = binaryGetUint64(buf[off:])
	off += 8
	c.CellType = CellType(buf[off])
	off++
	c.State = CellState(buf[off])
	off++
	c.Activity = binaryGetUint16(buf[off:])
	off += 2
	c.Connectivity = binaryGetUint16(buf[off:])
	off += 2
	c.Plasticity = buf[off]
	off++
	c.Age = binaryGetUint16(buf[off:])
	off += 2
	c.MaxAge = binaryGetUint16(buf[off:])
	off += 2
	c.IsAlive = buf[off] == 1
	off++
	c.LastUpdateTick = uint32(binaryGetUint32(buf[off:]))
	off += 4
	c.RegionID = binaryGetUint16(buf[off:])
	off += 2
	c.ClusterID = binaryGetUint16(buf[off:])
	off += 2
	c.Energy = binaryGetUint16(buf[off:])
	off += 2
	c.MaxEnergy = binaryGetUint16(buf[off:])
	off += 2
	c.FireCount = binaryGetUint32(buf[off:])
	off += 4
	c.WeightSum = binaryGetUint32(buf[off:])
	off += 4
	return true
}

func CellSize() int {
	return 44
}

func binaryPutUint16(buf []byte, v uint16) {
	buf[0] = byte(v >> 8)
	buf[1] = byte(v)
}

func binaryGetUint16(buf []byte) uint16 {
	return uint16(buf[0])<<8 | uint16(buf[1])
}

func binaryPutUint32(buf []byte, v uint32) {
	buf[0] = byte(v >> 24)
	buf[1] = byte(v >> 16)
	buf[2] = byte(v >> 8)
	buf[3] = byte(v)
}

func binaryGetUint32(buf []byte) uint32 {
	return uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
}

func binaryPutUint64(buf []byte, v uint64) {
	buf[0] = byte(v >> 56)
	buf[1] = byte(v >> 48)
	buf[2] = byte(v >> 40)
	buf[3] = byte(v >> 32)
	buf[4] = byte(v >> 24)
	buf[5] = byte(v >> 16)
	buf[6] = byte(v >> 8)
	buf[7] = byte(v)
}

func binaryGetUint64(buf []byte) uint64 {
	return uint64(buf[0])<<56 | uint64(buf[1])<<48 | uint64(buf[2])<<40 |
		uint64(buf[3])<<32 | uint64(buf[4])<<24 | uint64(buf[5])<<16 |
		uint64(buf[6])<<8 | uint64(buf[7])
}
