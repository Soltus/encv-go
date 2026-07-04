package simverse

import (
	"math/rand"
	"testing"
)

func TestChronicle_RecordAndQuery(t *testing.T) {
	cm := NewChronicleManager("test")

	if cm.Count() != 0 {
		t.Errorf("Expected 0 events, got %d", cm.Count())
	}

	id1 := cm.RecordPersonal(100, 1, ChronEventBirth, 0)
	id2 := cm.RecordPersonal(200, 1, ChronEventComingOfAge, 0)
	id3 := cm.RecordPersonal(300, 1, ChronEventFirstJob, 0)
	id4 := cm.RecordWorld(500, ChronEventGreatDisaster, 3)

	if cm.Count() != 4 {
		t.Errorf("Expected 4 events, got %d", cm.Count())
	}

	evt, ok := cm.GetEvent(id1)
	if !ok {
		t.Fatal("Event not found")
	}
	if evt.Type != ChronEventBirth {
		t.Errorf("Expected Birth, got %s", evt.Type)
	}
	if evt.EntityID != 1 {
		t.Errorf("Expected entity 1, got %d", evt.EntityID)
	}
	if evt.Importance != ImpModerate {
		t.Errorf("Expected Moderate importance, got %d", evt.Importance)
	}

	evt4, ok := cm.GetEvent(id4)
	if !ok {
		t.Fatal("World event not found")
	}
	if evt4.Level != ChronLevelWorld {
		t.Errorf("Expected World level, got %s", evt4.Level)
	}
	if evt4.Importance != ImpEpic {
		t.Errorf("Expected Epic importance, got %d", evt4.Importance)
	}

	npcHist := cm.NPCHistory(1, 10)
	if len(npcHist) != 3 {
		t.Errorf("Expected 3 NPC history events, got %d", len(npcHist))
	}
	if npcHist[0].Tick < npcHist[len(npcHist)-1].Tick {
		t.Error("Expected descending order")
	}

	worldTimeline := cm.WorldTimeline(ImpModerate, 10)
	if len(worldTimeline) != 1 {
		t.Errorf("Expected 1 world event, got %d", len(worldTimeline))
	}
	_ = id2
	_ = id3
}

func TestChronicle_QueryFilters(t *testing.T) {
	cm := NewChronicleManager("test")

	for i := 0; i < 10; i++ {
		tick := uint32(i * 100)
		imp := ChronicleImportance(i % int(ImpMax))
		cm.Record(ChronicleEvent{
			Tick:       tick,
			Type:       ChronEventAging,
			Importance: imp,
			EntityID:   100,
		})
	}

	if cm.Count() != 10 {
		t.Errorf("Expected 10, got %d", cm.Count())
	}

	filtered := cm.Query(ChronicleQuery{
		EntityID:      100,
		MinImportance: ImpMajor,
	})
	t.Logf("Events with >= Major importance: %d", len(filtered))
	for _, e := range filtered {
		if e.Importance < ImpMajor {
			t.Errorf("Event %d has importance %d < Major", e.ID, e.Importance)
		}
	}

	byTime := cm.Query(ChronicleQuery{
		FromTick:   300,
		ToTick:     700,
		EntityID:   100,
		Descending: false,
	})
	t.Logf("Events between tick 300-700: %d", len(byTime))
	for _, e := range byTime {
		if e.Tick < 300 || e.Tick > 700 {
			t.Errorf("Event %d tick %d out of range", e.ID, e.Tick)
		}
	}

	limited := cm.Query(ChronicleQuery{
		EntityID:   100,
		Limit:      3,
		Descending: true,
	})
	if len(limited) != 3 {
		t.Errorf("Expected 3 limited events, got %d", len(limited))
	}
}

func TestChronicle_Levels(t *testing.T) {
	cm := NewChronicleManager("test")

	cm.RecordPersonal(100, 1, ChronEventBirth, 0)
	cm.Record(ChronicleEvent{Tick: 200, Type: ChronEventFamilyFounded, EntityID: 1000})
	cm.RecordOrg(300, 50, ChronEventOrgFounded, 0)
	cm.RecordRegion(400, 5, ChronEventCityFounded)
	cm.RecordWorld(500, ChronEventEraStart, 1)

	if cm.CountByLevel(ChronLevelPersonal) != 1 {
		t.Errorf("Expected 1 personal, got %d", cm.CountByLevel(ChronLevelPersonal))
	}
	if cm.CountByLevel(ChronLevelFamily) != 1 {
		t.Errorf("Expected 1 family, got %d", cm.CountByLevel(ChronLevelFamily))
	}
	if cm.CountByLevel(ChronLevelOrg) != 1 {
		t.Errorf("Expected 1 org, got %d", cm.CountByLevel(ChronLevelOrg))
	}
	if cm.CountByLevel(ChronLevelRegion) != 1 {
		t.Errorf("Expected 1 region, got %d", cm.CountByLevel(ChronLevelRegion))
	}
	if cm.CountByLevel(ChronLevelWorld) != 1 {
		t.Errorf("Expected 1 world, got %d", cm.CountByLevel(ChronLevelWorld))
	}
}

func TestChronicle_CauseAndEffect(t *testing.T) {
	cm := NewChronicleManager("test")

	id1 := cm.RecordWorld(100, ChronEventGreatDisaster, 0)
	id2 := cm.Record(ChronicleEvent{
		Tick:     110,
		Type:     ChronEventMigration,
		Level:    ChronLevelRegion,
		Importance: ImpMajor,
		Cause1ID: id1,
	})
	id3 := cm.Record(ChronicleEvent{
		Tick:     120,
		Type:     ChronEventEconomyRecess,
		Level:    ChronLevelRegion,
		Importance: ImpMajor,
		Cause1ID: id1,
		Cause2ID: id2,
	})

	causes := cm.Causes(id3)
	if len(causes) != 2 {
		t.Errorf("Expected 2 causes, got %d", len(causes))
	}

	effects := cm.Effects(id1, 10)
	if len(effects) != 2 {
		t.Errorf("Expected 2 effects of event 1, got %d", len(effects))
	}

	t.Logf("Event %d has %d causes, %d effects", id3, len(causes), len(effects))
}

func TestChronicle_EraManagement(t *testing.T) {
	cm := NewChronicleManager("test")

	cm.SetEra(1, 0, "First Era")
	if cm.CurrentEra() != 1 {
		t.Errorf("Expected era 1, got %d", cm.CurrentEra())
	}

	cm.SetEra(2, 1000, "Second Era")
	if cm.CurrentEra() != 2 {
		t.Errorf("Expected era 2, got %d", cm.CurrentEra())
	}

	worldEvents := cm.WorldTimeline(ImpEpic, 10)
	t.Logf("World timeline events: %d", len(worldEvents))
	for _, e := range worldEvents {
		t.Logf("  tick=%d type=%s imp=%s", e.Tick, e.Type, e.Importance)
	}

	eraStarts := 0
	eraEnds := 0
	for _, e := range worldEvents {
		if e.Type == ChronEventEraStart {
			eraStarts++
		}
		if e.Type == ChronEventEraEnd {
			eraEnds++
		}
	}
	if eraStarts != 2 {
		t.Errorf("Expected 2 era starts, got %d", eraStarts)
	}
	if eraEnds != 1 {
		t.Errorf("Expected 1 era end, got %d", eraEnds)
	}
}

func TestChronicle_GenerateWorldHistory(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	cm := NewChronicleManager("test")

	cm.GenerateWorldHistory(rng, 0, 3)

	stats := cm.MemoryStats()
	t.Logf("Generated world history: %.0f events", stats["event_count"])
	t.Logf("  Personal: %.0f", stats["personal_count"])
	t.Logf("  World: %.0f", stats["world_count"])
	t.Logf("  Size: %.0f bytes", stats["total_bytes"])

	if cm.CurrentEra() != 3 {
		t.Errorf("Expected 3 eras, got %d", cm.CurrentEra())
	}
	if stats["world_count"] < 10 {
		t.Errorf("Expected at least 10 world events, got %.0f", stats["world_count"])
	}
}

func TestChronicle_MemoryStats(t *testing.T) {
	cm := NewChronicleManager("test")

	for i := 0; i < 100; i++ {
		cm.RecordPersonal(uint32(i*10), uint64(i%10), ChronEventAging, 0)
	}

	stats := cm.MemoryStats()
	t.Logf("Chronicle stats:")
	t.Logf("  event_count: %.0f", stats["event_count"])
	t.Logf("  total_bytes: %.0f", stats["total_bytes"])
	t.Logf("  personal_count: %.0f", stats["personal_count"])
	t.Logf("  entity_indexed: %.0f", stats["entity_indexed"])

	expectedBytes := 100 * ChronicleEventSizeEstimate()
	if stats["total_bytes"] < expectedBytes*0.9 || stats["total_bytes"] > expectedBytes*1.1 {
		t.Logf("Warning: expected ~%.0f bytes, got %.0f", expectedBytes, stats["total_bytes"])
	}
}

func TestChronicle_EventTypeLevel(t *testing.T) {
	tests := []struct {
		evtType ChronicleEventType
		level   ChronicleLevel
	}{
		{ChronEventBirth, ChronLevelPersonal},
		{ChronEventDeath, ChronLevelPersonal},
		{ChronEventFamilyFounded, ChronLevelFamily},
		{ChronEventOrgFounded, ChronLevelOrg},
		{ChronEventCityFounded, ChronLevelRegion},
		{ChronEventEraStart, ChronLevelWorld},
	}

	for _, tt := range tests {
		if tt.evtType.Level() != tt.level {
			t.Errorf("%s: expected level %s, got %s", tt.evtType, tt.level, tt.evtType.Level())
		}
	}
}

func TestChronicle_DefaultImportance(t *testing.T) {
	tests := []struct {
		evtType ChronicleEventType
		imp     ChronicleImportance
	}{
		{ChronEventBirth, ImpModerate},
		{ChronEventDeath, ImpModerate},
		{ChronEventAging, ImpTrivial},
		{ChronEventPromotion, ImpMinor},
		{ChronEventOrgFounded, ImpMajor},
		{ChronEventCityDestroyed, ImpGreat},
		{ChronEventEraStart, ImpEpic},
		{ChronEventGreatDisaster, ImpEpic},
	}

	for _, tt := range tests {
		imp := DefaultImportanceForType(tt.evtType)
		if imp != tt.imp {
			t.Errorf("%s: expected importance %s, got %s", tt.evtType, tt.imp, imp)
		}
	}
}
