package simverse

import "math"

// ============================================================
// 记忆系统 — 短期记忆 + 长期记忆 + 遗忘曲线
//
// 设计原则：
//   - 短期记忆：环形 buffer，固定容量，先进先出
//   - 长期记忆：按强度衰减，重要事件强度高不易忘
//   - 只存短期记忆在 NPC 结构体里（省内存）
//   - 长期记忆按需从事件日志重建（存储分层）
//   - 记忆类型：交互/事实/情绪/成就/创伤
// ============================================================

type MemoryType uint8

const (
	MemInteraction MemoryType = 0
	MemFact        MemoryType = 1
	MemEmotion     MemoryType = 2
	MemAchievement MemoryType = 3
	MemTrauma      MemoryType = 4
	MemMax                    = 5
)

var MemoryTypeNames = [MemMax]string{
	"Interaction",
	"Fact",
	"Emotion",
	"Achievement",
	"Trauma",
}

var MemoryTypeCNNames = [MemMax]string{
	"交互",
	"事实",
	"情绪",
	"成就",
	"创伤",
}

func (m MemoryType) String() string {
	if int(m) < int(MemMax) {
		return MemoryTypeNames[m]
	}
	return "Unknown"
}

func (m MemoryType) CN() string {
	if int(m) < int(MemMax) {
		return MemoryTypeCNNames[m]
	}
	return "未知"
}

type MemoryImportance uint8

const (
	ImpTrivial   MemoryImportance = 0
	ImpMinor     MemoryImportance = 1
	ImpModerate  MemoryImportance = 2
	ImpMajor     MemoryImportance = 3
	ImpCritical  MemoryImportance = 4
	ImpMax                       = 5
)

type Memory struct {
	ID         uint32
	Type       MemoryType
	Importance MemoryImportance
	TargetID   uint64
	ContentTag uint16
	EmotionTag int8
	CreatedTick uint32
	Strength   float32
}

const ShortTermMemoryCapacity = 20

type ShortTermMemory struct {
	items  [ShortTermMemoryCapacity]Memory
	head   uint8
	count  uint8
}

func NewShortTermMemory() ShortTermMemory {
	return ShortTermMemory{}
}

func (stm *ShortTermMemory) Add(mem Memory) {
	if stm.count < ShortTermMemoryCapacity {
		stm.items[stm.count] = m
		stm.count++
	} else {
		stm.items[stm.head] = m
		stm.head = (stm.head + 1) % ShortTermMemoryCapacity
	}
}

func (stm *ShortTermMemory) Items() []Memory {
	result := make([]Memory, 0, stm.count)
	for i := uint8(0); i < stm.count; i++ {
		idx := (stm.head + i) % ShortTermMemoryCapacity
		result = append(result, stm.items[idx])
	}
	return result
}

func (stm *ShortTermMemory) Recent(n int) []Memory {
	all := stm.Items()
	if n <= 0 || n >= len(all) {
		return all
	}
	return all[len(all)-n:]
}

func (stm *ShortTermMemory) Count() int {
	return int(stm.count)
}

func (stm *ShortTermMemory) ByType(t MemoryType) []Memory {
	result := make([]Memory, 0, stm.count)
	for i := uint8(0); i < stm.count; i++ {
		idx := (stm.head + i) % ShortTermMemoryCapacity
		if stm.items[idx].Type == t {
			result = append(result, stm.items[idx])
		}
	}
	return result
}

type LongTermMemory struct {
	memories []Memory
}

func NewLongTermMemory() LongTermMemory {
	return LongTermMemory{
		memories: make([]Memory, 0, 32),
	}
}

func (ltm *LongTermMemory) Add(m Memory) {
	baseStrength := float32(0.0)
	switch m.Importance {
	case ImpTrivial:
		baseStrength = 0.3
	case ImpMinor:
		baseStrength = 0.5
	case ImpModerate:
		baseStrength = 0.7
	case ImpMajor:
		baseStrength = 0.85
	case ImpCritical:
		baseStrength = 0.95
	}
	m.Strength = baseStrength
	ltm.memories = append(ltm.memories, m)
}

func (ltm *LongTermMemory) Decay(currentTick uint32, decayRate float32) {
	for i := range ltm.memories {
		m := &ltm.memories[i]
		elapsed := float32(currentTick - m.CreatedTick)
		if elapsed <= 0 {
			continue
		}

		baseStr := float32(0.0)
		switch m.Importance {
		case ImpTrivial:
			baseStr = 0.3
		case ImpMinor:
			baseStr = 0.5
		case ImpModerate:
			baseStr = 0.7
		case ImpMajor:
			baseStr = 0.85
		case ImpCritical:
			baseStr = 0.95
		}

		halfLife := float32(100.0) * (1 + float32(m.Importance)*2)
		m.Strength = baseStr * float32(math.Exp2(float64(-elapsed/(halfLife*(1.0+decayRate))))
		if m.Strength < 0.01 {
			m.Strength = 0.01
		}
	}

	threshold := float32(0.05)
	filtered := ltm.memories[:0]
	for _, m := range ltm.memories {
		if m.Strength > threshold {
			filtered = append(filtered, m)
		}
	}
	ltm.memories = filtered
}

func (ltm *LongTermMemory) Recall(query string, currentTick uint32) []Memory {
	result := make([]Memory, 0, len(ltm.memories))
	for _, m := range ltm.memories {
		elapsed := currentTick - m.CreatedTick
		recencyBonus := float32(0)
		if elapsed < 100 {
			recencyBonus = 0.1
		}
		if m.Strength+recencyBonus > 0.3 {
			result = append(result, m)
		}
	}
	return result
}

func (ltm *LongTermMemory) ByType(t MemoryType) []Memory {
	result := make([]Memory, 0, len(ltm.memories))
	for _, m := range ltm.memories {
		if m.Type == t {
			result = append(result, m)
		}
	}
	return result
}

func (ltm *LongTermMemory) Count() int {
	return len(ltm.memories)
}

func (ltm *LongTermMemory) Items() []Memory {
	result := make([]Memory, len(ltm.memories))
	copy(result, ltm.memories)
	return result
}

type MemorySystem struct {
	ShortTerm ShortTermMemory
	LongTerm  LongTermMemory
}

func NewMemorySystem() MemorySystem {
	return MemorySystem{
		ShortTerm: NewShortTermMemory(),
		LongTerm:  NewLongTermMemory(),
	}
}

func (ms *MemorySystem) Record(m Memory) {
	ms.ShortTerm.Add(m)
	if m.Importance >= ImpModerate {
		ms.LongTerm.Add(m)
	}
}

func (ms *MemorySystem) Decay(currentTick uint32) {
	ms.LongTerm.Decay(currentTick, 0)
}

func (ms *MemorySystem) TotalCount() int {
	return ms.ShortTerm.Count() + ms.LongTerm.Count()
}

func ShortTermMemorySize() float64 {
	return float64(ShortTermMemoryCapacity) * 24
}

func MemorySize() float64 {
	return 24
}
