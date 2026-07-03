package simverse

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// ============================================================
// 编年史管理器 — 五级时间线存储 + 多维查询
//
// 索引结构：
//   - events:         所有事件按时间排序的主列表
//   - byEntity:       按实体 ID 索引（NPC / 组织 / 区域）
//   - byLevel:        按编年史层级索引（0-4）
//   - byType:         按事件类型索引
// ============================================================

type ChronicleManager struct {
	mu         sync.RWMutex
	events     []ChronicleEvent
	nextID     uint64
	byEntity   map[uint64][]uint64
	byLevel    [ChronLevelMax][]uint64
	byType     map[ChronicleEventType][]uint64
	worldEra   uint16
	worldName  string
}

func NewChronicleManager(worldName string) *ChronicleManager {
	return &ChronicleManager{
		events:    make([]ChronicleEvent, 0, 1024),
		nextID:    1,
		byEntity:  make(map[uint64][]uint64),
		byType:    make(map[ChronicleEventType][]uint64),
		worldName: worldName,
	}
}

func (cm *ChronicleManager) Record(evt ChronicleEvent) uint64 {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if evt.ID == 0 {
		evt.ID = cm.nextID
		cm.nextID++
	}
	evt.Level = evt.Type.Level()
	if evt.Importance == 0 {
		evt.Importance = DefaultImportanceForType(evt.Type)
	}

	cm.events = append(cm.events, evt)

	cm.byEntity[evt.EntityID] = append(cm.byEntity[evt.EntityID], evt.ID)
	if evt.Level < ChronLevelMax {
		cm.byLevel[evt.Level] = append(cm.byLevel[evt.Level], evt.ID)
	}
	cm.byType[evt.Type] = append(cm.byType[evt.Type], evt.ID)

	return evt.ID
}

func (cm *ChronicleManager) RecordPersonal(tick uint32, npcID uint64, evtType ChronicleEventType, targetID uint64) uint64 {
	return cm.Record(ChronicleEvent{
		Tick:       tick,
		Type:       evtType,
		EntityID:   npcID,
		TargetID:   targetID,
	})
}

func (cm *ChronicleManager) RecordWorld(tick uint32, evtType ChronicleEventType, dataTag uint16) uint64 {
	return cm.Record(ChronicleEvent{
		Tick:     tick,
		Type:     evtType,
		DataTag:  dataTag,
	})
}

func (cm *ChronicleManager) RecordOrg(tick uint32, orgID uint64, evtType ChronicleEventType, targetID uint64) uint64 {
	return cm.Record(ChronicleEvent{
		Tick:     tick,
		Type:     evtType,
		EntityID: orgID,
		TargetID: targetID,
	})
}

func (cm *ChronicleManager) RecordRegion(tick uint32, regionID uint32, evtType ChronicleEventType) uint64 {
	return cm.Record(ChronicleEvent{
		Tick:     tick,
		Type:     evtType,
		EntityID: uint64(regionID),
	})
}

func (cm *ChronicleManager) GetEvent(id uint64) (ChronicleEvent, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if id == 0 || id >= cm.nextID {
		return ChronicleEvent{}, false
	}
	if int(id) > len(cm.events) {
		return ChronicleEvent{}, false
	}
	return cm.events[id-1], true
}

func (cm *ChronicleManager) Count() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.events)
}

func (cm *ChronicleManager) CountByLevel(level ChronicleLevel) int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if level >= ChronLevelMax {
		return 0
	}
	return len(cm.byLevel[level])
}

func (cm *ChronicleManager) CountByEntity(entityID uint64) int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.byEntity[entityID])
}

type ChronicleQuery struct {
	FromTick      uint32
	ToTick        uint32
	EntityID      uint64
	HasEntity     bool
	Level         ChronicleLevel
	HasLevel      bool
	EventType     ChronicleEventType
	MinImportance ChronicleImportance
	Limit         int
	Descending    bool
}

func (cm *ChronicleManager) Query(q ChronicleQuery) []ChronicleEvent {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var candidateIDs []uint64

	switch {
	case q.HasEntity:
		candidateIDs = cm.byEntity[q.EntityID]
	case q.EventType != 0:
		candidateIDs = cm.byType[q.EventType]
	case q.HasLevel:
		candidateIDs = cm.byLevel[q.Level]
	default:
		candidateIDs = make([]uint64, len(cm.events))
		for i := range cm.events {
			candidateIDs[i] = cm.events[i].ID
		}
	}

	results := make([]ChronicleEvent, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		if id == 0 || id > uint64(len(cm.events)) {
			continue
		}
		evt := cm.events[id-1]

		if q.FromTick > 0 && evt.Tick < q.FromTick {
			continue
		}
		if q.ToTick > 0 && evt.Tick > q.ToTick {
			continue
		}
		if q.MinImportance > 0 && evt.Importance < q.MinImportance {
			continue
		}
		if q.EventType != 0 && evt.Type != q.EventType {
			continue
		}

		results = append(results, evt)

		if q.Limit > 0 && len(results) >= q.Limit {
			break
		}
	}

	if q.Descending {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Tick > results[j].Tick
		})
	} else {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Tick < results[j].Tick
		})
	}

	return results
}

func (cm *ChronicleManager) WorldTimeline(minImportance ChronicleImportance, limit int) []ChronicleEvent {
	return cm.Query(ChronicleQuery{
		Level:         ChronLevelWorld,
		HasLevel:      true,
		MinImportance: minImportance,
		Limit:         limit,
		Descending:    true,
	})
}

func (cm *ChronicleManager) NPCHistory(npcID uint64, limit int) []ChronicleEvent {
	return cm.Query(ChronicleQuery{
		EntityID:   npcID,
		HasEntity:  true,
		Limit:      limit,
		Descending: true,
	})
}

func (cm *ChronicleManager) OrgHistory(orgID uint64, limit int) []ChronicleEvent {
	return cm.Query(ChronicleQuery{
		EntityID:   orgID,
		HasEntity:  true,
		Limit:      limit,
		Descending: true,
	})
}

func (cm *ChronicleManager) RegionHistory(regionID uint32, limit int) []ChronicleEvent {
	return cm.Query(ChronicleQuery{
		EntityID:   uint64(regionID),
		HasEntity:  true,
		Limit:      limit,
		Descending: true,
	})
}

func (cm *ChronicleManager) Causes(evtID uint64) []ChronicleEvent {
	evt, ok := cm.GetEvent(evtID)
	if !ok {
		return nil
	}

	causes := make([]ChronicleEvent, 0, 3)
	for _, cid := range []uint64{evt.Cause1ID, evt.Cause2ID, evt.Cause3ID} {
		if cid == 0 {
			continue
		}
		if c, ok := cm.GetEvent(cid); ok {
			causes = append(causes, c)
		}
	}
	return causes
}

func (cm *ChronicleManager) Effects(evtID uint64, limit int) []ChronicleEvent {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	effects := make([]ChronicleEvent, 0, limit)
	for _, e := range cm.events {
		if e.Cause1ID == evtID || e.Cause2ID == evtID || e.Cause3ID == evtID {
			effects = append(effects, e)
			if len(effects) >= limit {
				break
			}
		}
	}
	return effects
}

func (cm *ChronicleManager) CurrentEra() uint16 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.worldEra
}

func (cm *ChronicleManager) SetEra(era uint16, tick uint32, name string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.worldEra == era {
		return
	}

	if cm.worldEra > 0 {
		cm.events = append(cm.events, ChronicleEvent{
			ID:         cm.nextID,
			Tick:       tick,
			Level:      ChronLevelWorld,
			Type:       ChronEventEraEnd,
			Importance: ImpEpic,
			DataTag:    uint16(cm.worldEra),
		})
		cm.nextID++
	}

	cm.worldEra = era

	cm.events = append(cm.events, ChronicleEvent{
		ID:         cm.nextID,
		Tick:       tick,
		Level:      ChronLevelWorld,
		Type:       ChronEventEraStart,
		Importance: ImpEpic,
		DataTag:    era,
	})
	cm.nextID++

	cm.byLevel[ChronLevelWorld] = append(cm.byLevel[ChronLevelWorld], cm.nextID-2, cm.nextID-1)
	cm.byType[ChronEventEraEnd] = append(cm.byType[ChronEventEraEnd], cm.nextID-2)
	cm.byType[ChronEventEraStart] = append(cm.byType[ChronEventEraStart], cm.nextID-1)
}

func (cm *ChronicleManager) GenerateWorldHistory(rng *rand.Rand, startTick uint32, eras int) {
	if eras <= 0 {
		eras = 3
	}

	tick := startTick
	for era := 0; era < eras; era++ {
		eraTicks := 500 + rng.Intn(1000)
		cm.SetEra(uint16(era+1), tick, "")

		numEvents := 3 + rng.Intn(5)
		for i := 0; i < numEvents; i++ {
			evtTick := tick + uint32(rng.Intn(eraTicks/2))
			imp := ChronicleImportance(2 + rng.Intn(3))
			evtTypes := []ChronicleEventType{
				ChronEventGreatDisaster,
				ChronEventGreatMiracle,
				ChronEventCivilizationRise,
				ChronEventCivilizationFall,
				ChronEventMagicTide,
			}
			evtType := evtTypes[rng.Intn(len(evtTypes))]
			cm.Record(ChronicleEvent{
				Tick:       evtTick,
				Level:      ChronLevelWorld,
				Type:       evtType,
				Importance: imp,
				DataTag:    uint16(rng.Intn(10)),
			})
		}

		tick += uint32(eraTicks)
	}
}

func (cm *ChronicleManager) MemoryStats() map[string]float64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	count := len(cm.events)
	return map[string]float64{
		"event_count":    float64(count),
		"total_bytes":    float64(count) * ChronicleEventSizeEstimate(),
		"personal_count": float64(len(cm.byLevel[ChronLevelPersonal])),
		"family_count":   float64(len(cm.byLevel[ChronLevelFamily])),
		"org_count":      float64(len(cm.byLevel[ChronLevelOrg])),
		"region_count":   float64(len(cm.byLevel[ChronLevelRegion])),
		"world_count":    float64(len(cm.byLevel[ChronLevelWorld])),
		"entity_indexed": float64(len(cm.byEntity)),
	}
}

type chronicleSaveData struct {
	NextID   uint64           `json:"next_id"`
	WorldEra uint16           `json:"world_era"`
	Events   []ChronicleEvent `json:"events"`
}

func (cm *ChronicleManager) SaveToFile(path string) error {
	cm.mu.RLock()
	data := chronicleSaveData{
		NextID:   cm.nextID,
		WorldEra: cm.worldEra,
		Events:   make([]ChronicleEvent, len(cm.events)),
	}
	copy(data.Events, cm.events)
	cm.mu.RUnlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func (cm *ChronicleManager) LoadFromFile(path string) error {
	jsonData, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var data chronicleSaveData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.nextID = data.NextID
	cm.worldEra = data.WorldEra
	cm.events = make([]ChronicleEvent, len(data.Events))
	copy(cm.events, data.Events)

	cm.byEntity = make(map[uint64][]uint64)
	cm.byLevel = [ChronLevelMax][]uint64{}
	cm.byType = make(map[ChronicleEventType][]uint64)

	for i := range cm.events {
		evt := &cm.events[i]
		if evt.ID == 0 {
			evt.ID = cm.nextID
			cm.nextID++
		}
		if evt.Level == 0 {
			evt.Level = evt.Type.Level()
		}
		if evt.Importance == 0 {
			evt.Importance = DefaultImportanceForType(evt.Type)
		}

		cm.byEntity[evt.EntityID] = append(cm.byEntity[evt.EntityID], evt.ID)
		if evt.Level < ChronLevelMax {
			cm.byLevel[evt.Level] = append(cm.byLevel[evt.Level], evt.ID)
		}
		cm.byType[evt.Type] = append(cm.byType[evt.Type], evt.ID)
	}

	return nil
}
