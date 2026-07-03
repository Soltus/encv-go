package simverse

import (
	"math/rand"
	"sync"
)

type FocusLevel int

const (
	FocusNone     FocusLevel = 0
	FocusDistant  FocusLevel = 1
	FocusNear     FocusLevel = 2
	FocusCore     FocusLevel = 3
	FocusPlayer   FocusLevel = 4
)

func (fl FocusLevel) String() string {
	switch fl {
	case FocusNone:
		return "none"
	case FocusDistant:
		return "distant"
	case FocusNear:
		return "near"
	case FocusCore:
		return "core"
	case FocusPlayer:
		return "player"
	default:
		return "unknown"
	}
}

type FractalWorld struct {
	mu sync.RWMutex

	worldTick     uint32
	perfTier      PerformanceTier
	perfSched     *PerfScheduler
	chronicle     *ChronicleManager

	npcCache      *EntityCache[*NPCV3]
	cellCache     map[uint64]*EntityCache[*Cell]
	brainCache    map[uint64]*Brain

	focusNPCs     map[uint64]FocusLevel
	cellCoreCount int
}

func NewFractalWorld() *FractalWorld {
	fw := &FractalWorld{
		perfSched:     NewPerfScheduler(),
		chronicle:     NewChronicleManager("world"),
		npcCache:      NewEntityCache[*NPCV3](10000),
		cellCache:     make(map[uint64]*EntityCache[*Cell]),
		brainCache:    make(map[uint64]*Brain),
		focusNPCs:     make(map[uint64]FocusLevel),
		cellCoreCount: 1000,
	}
	return fw
}

func (fw *FractalWorld) SetPerformanceTier(tier PerformanceTier) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.perfTier = tier
	fw.perfSched.SetTier(tier)

	config := PerfTiers[tier]
	fw.npcCache.Resize(config.CacheSize)
}

func (fw *FractalWorld) AddFocusNPC(npcID uint64, level FocusLevel) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.focusNPCs[npcID] = level
	fw.perfSched.AddFocusNPC(npcID, uint8(level))
}

func (fw *FractalWorld) RemoveFocusNPC(npcID uint64) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	delete(fw.focusNPCs, npcID)
	fw.perfSched.RemoveFocusNPC(npcID)
}

func (fw *FractalWorld) GetNPC(npcID uint64, rng *rand.Rand) *NPCV3 {
	fw.mu.RLock()
	if npc, ok := fw.npcCache.Get(npcID); ok {
		fw.mu.RUnlock()
		return npc
	}
	currentTick := fw.worldTick
	chron := fw.chronicle
	fw.mu.RUnlock()

	npc := fw.generateNPC(npcID, rng)
	npc.BornAt = int32(currentTick)

	chron.RecordPersonal(currentTick, npcID, ChronEventBirth, 0)

	fw.mu.Lock()
	fw.npcCache.Put(npcID, npc)
	fw.mu.Unlock()
	return npc
}

func (fw *FractalWorld) GetBrain(npcID uint64) *Brain {
	fw.mu.RLock()
	if brain, ok := fw.brainCache[npcID]; ok {
		fw.mu.RUnlock()
		return brain
	}
	fw.mu.RUnlock()

	brain := NewBrain(npcID)
	fw.mu.Lock()
	fw.brainCache[npcID] = brain
	if _, ok := fw.cellCache[npcID]; !ok {
		fw.cellCache[npcID] = NewEntityCache[*Cell](fw.cellCoreCount)
	}
	fw.mu.Unlock()
	return brain
}

func (fw *FractalWorld) GetCells(npcID uint64, region BrainRegion, count int, rng *rand.Rand) []Cell {
	brain := fw.GetBrain(npcID)
	npc := fw.GetNPC(npcID, rng)
	return brain.GetActiveCells(region, count, npc.Age, rng)
}

func (fw *FractalWorld) Tick(rng *rand.Rand) {
	fw.mu.Lock()
	tier := fw.perfTier
	config := PerfTiers[tier]
	fw.worldTick++
	currentTick := fw.worldTick
	chron := fw.chronicle
	fw.mu.Unlock()

	fw.tickNPCs(currentTick, config, rng)
	fw.tickBrains(currentTick, config, rng)
	fw.tickWorldEvents(currentTick, chron, rng)
}

func (fw *FractalWorld) tickWorldEvents(currentTick uint32, chron *ChronicleManager, rng *rand.Rand) {
	if currentTick%100 != 0 {
		return
	}

	if rng.Float64() < 0.02 {
		evtTypes := []ChronicleEventType{
			ChronEventGreatDisaster,
			ChronEventDisaster,
			ChronEventMigration,
			ChronEventEconomyBoom,
			ChronEventEconomyRecess,
		}
		evt := evtTypes[rng.Intn(len(evtTypes))]
		chron.RecordWorld(currentTick, evt, uint16(rng.Intn(100)))
	}

	if currentTick%10000 == 0 {
		era := chron.CurrentEra()
		chron.SetEra(era+1, currentTick, "")
	}
}

func (fw *FractalWorld) tickNPCs(currentTick uint32, config PerfTierConfig, rng *rand.Rand) {
	fw.mu.RLock()
	npcs := fw.npcCache.All()
	chron := fw.chronicle
	fw.mu.RUnlock()

	eventRate := config.EventRateMul
	if eventRate < 1.0 {
		for _, npc := range npcs {
			if rng.Float64() < eventRate {
				result := npc.CatchUp(currentTick, rng)
				fw.recordChronicleEvents(chron, npc, currentTick, result)
			}
		}
	} else {
		extraTicks := int(eventRate)
		for i := 0; i < extraTicks; i++ {
			for _, npc := range npcs {
				result := npc.CatchUp(currentTick, rng)
				fw.recordChronicleEvents(chron, npc, currentTick, result)
			}
		}
	}
}

func (fw *FractalWorld) recordChronicleEvents(chron *ChronicleManager, npc *NPCV3, tick uint32, result CatchUpResult) {
	if len(result.LifeEvents) == 0 {
		return
	}
	for _, evt := range result.LifeEvents {
		chronType := LifeEventToChronicle(evt)
		chron.Record(ChronicleEvent{
			Tick:     tick,
			Type:     chronType,
			EntityID: npc.ID,
		})
	}
}

func (fw *FractalWorld) tickBrains(currentTick uint32, config PerfTierConfig, rng *rand.Rand) {
	if !config.SubSimActive {
		return
	}

	fw.mu.RLock()
	focusIDs := make([]uint64, 0, len(fw.focusNPCs))
	for id, level := range fw.focusNPCs {
		if int(level) >= config.SubSimDepth {
			focusIDs = append(focusIDs, id)
		}
	}
	fw.mu.RUnlock()

	for _, npcID := range focusIDs {
		brain := fw.GetBrain(npcID)
		npc := fw.GetNPC(npcID, rng)
		brain.CatchUp(currentTick, rng, npc)
	}
}

func (fw *FractalWorld) generateNPC(npcID uint64, rng *rand.Rand) *NPCV3 {
	return GenerateNPCV3(npcID, rng)
}

func (fw *FractalWorld) MemoryStats() map[string]float64 {
	fw.mu.RLock()
	defer fw.mu.RUnlock()

	stats := make(map[string]float64)

	npcCount := fw.npcCache.Len()
	stats["npc_cache_count"] = float64(npcCount)
	stats["npc_cache_mb"] = float64(npcCount) * NPCV3Size() / 1024 / 1024

	totalCells := 0
	for _, cc := range fw.cellCache {
		totalCells += cc.Len()
	}
	stats["cell_cache_count"] = float64(totalCells)
	stats["cell_cache_mb"] = float64(totalCells) * float64(CellSize()) / 1024 / 1024

	stats["brain_cache_count"] = float64(len(fw.brainCache))
	stats["brain_cache_mb"] = float64(len(fw.brainCache)) * 256 / 1024

	stats["focus_npc_count"] = float64(len(fw.focusNPCs))

	chronStats := fw.chronicle.MemoryStats()
	for k, v := range chronStats {
		stats["chron_"+k] = v
	}
	stats["chron_mb"] = chronStats["total_bytes"] / 1024 / 1024

	stats["total_mb"] = stats["npc_cache_mb"] + stats["cell_cache_mb"] + stats["brain_cache_mb"] + stats["chron_mb"]

	return stats
}

func (fw *FractalWorld) Chronicle() *ChronicleManager {
	return fw.chronicle
}

func (fw *FractalWorld) WorldTick() uint32 {
	fw.mu.RLock()
	defer fw.mu.RUnlock()
	return fw.worldTick
}

func (fw *FractalWorld) CurrentTier() PerformanceTier {
	fw.mu.RLock()
	defer fw.mu.RUnlock()
	return fw.perfTier
}

func (fw *FractalWorld) PerfTierName() string {
	tier := fw.CurrentTier()
	switch tier {
	case PerfTierBackground:
		return "background"
	case PerfTierForeground:
		return "foreground"
	case PerfTierFgIdle:
		return "fg_idle"
	default:
		return "unknown"
	}
}

func (fw *FractalWorld) ListFocusNPCs() []uint64 {
	fw.mu.RLock()
	defer fw.mu.RUnlock()
	ids := make([]uint64, 0, len(fw.focusNPCs))
	for id := range fw.focusNPCs {
		ids = append(ids, id)
	}
	return ids
}

func (fw *FractalWorld) FocusLevel(npcID uint64) FocusLevel {
	fw.mu.RLock()
	defer fw.mu.RUnlock()
	if level, ok := fw.focusNPCs[npcID]; ok {
		return level
	}
	return FocusNone
}
