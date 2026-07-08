package simverse

import (
	"math/rand"
	"testing"
)

func TestGeneratedEventType_String(t *testing.T) {
	tests := []struct {
		et       GeneratedEventType
		expected string
	}{
		{GenEventPersonalIllness, "personal_illness"},
		{GenEventPersonalPromotion, "personal_promotion"},
		{GenEventPersonalLearnSkill, "personal_learn_skill"},
		{GenEventPersonalShopping, "personal_shopping"},
		{GenEventPersonalAccident, "personal_accident"},
		{GenEventInteractionConversation, "interaction_conversation"},
		{GenEventInteractionTrade, "interaction_trade"},
		{GenEventInteractionConflict, "interaction_conflict"},
		{GenEventInteractionCooperation, "interaction_cooperation"},
		{GenEventInteractionFriendship, "interaction_friendship"},
		{GenEventOrgMeeting, "org_meeting"},
		{GenEventOrgElection, "org_election"},
		{GenEventRegionFestival, "region_festival"},
		{GenEventRegionDisaster, "region_disaster"},
		{GenEventRegionMarket, "region_market"},
		{GenEventTypeMax, "unknown"},
	}

	for _, tt := range tests {
		if got := tt.et.String(); got != tt.expected {
			t.Errorf("GeneratedEventType(%d).String() = %q, want %q", tt.et, got, tt.expected)
		}
	}
}

func TestGeneratedEventType_CN(t *testing.T) {
	tests := []struct {
		et       GeneratedEventType
		expected string
	}{
		{GenEventPersonalIllness, "生病"},
		{GenEventPersonalPromotion, "晋升"},
		{GenEventInteractionConversation, "对话交流"},
		{GenEventOrgMeeting, "组织会议"},
		{GenEventRegionFestival, "地区节日"},
	}

	for _, tt := range tests {
		if got := tt.et.CN(); got != tt.expected {
			t.Errorf("GeneratedEventType(%d).CN() = %q, want %q", tt.et, got, tt.expected)
		}
	}
}

func TestGeneratedEventType_Category(t *testing.T) {
	tests := []struct {
		et       GeneratedEventType
		expected string
	}{
		{GenEventPersonalIllness, "personal"},
		{GenEventPersonalPromotion, "personal"},
		{GenEventPersonalLearnSkill, "personal"},
		{GenEventPersonalShopping, "personal"},
		{GenEventPersonalAccident, "personal"},
		{GenEventInteractionConversation, "interaction"},
		{GenEventInteractionTrade, "interaction"},
		{GenEventInteractionConflict, "interaction"},
		{GenEventInteractionCooperation, "interaction"},
		{GenEventInteractionFriendship, "interaction"},
		{GenEventOrgMeeting, "org"},
		{GenEventOrgElection, "org"},
		{GenEventRegionFestival, "region"},
		{GenEventRegionDisaster, "region"},
		{GenEventRegionMarket, "region"},
	}

	for _, tt := range tests {
		if got := tt.et.Category(); got != tt.expected {
			t.Errorf("GeneratedEventType(%d).Category() = %q, want %q", tt.et, got, tt.expected)
		}
	}
}

func TestGeneratedEventType_BaseWeight(t *testing.T) {
	for i := 0; i < int(GenEventTypeMax); i++ {
		et := GeneratedEventType(i)
		w := et.BaseWeight()
		if w <= 0 {
			t.Errorf("GeneratedEventType(%d).BaseWeight() = %d, want > 0", i, w)
		}
	}
}

func TestNewEventGenerator(t *testing.T) {
	eg := NewEventGenerator()
	if eg == nil {
		t.Fatal("NewEventGenerator() returned nil")
	}
	if eg.baseRate <= 0 {
		t.Errorf("baseRate should be positive, got %f", eg.baseRate)
	}
}

func TestEventGenerator_SetRates(t *testing.T) {
	eg := NewEventGenerator()
	eg.SetRates(0.05, 2.0, 1.5, 1.0, 0.5)

	if eg.baseRate != 0.05 {
		t.Errorf("expected baseRate 0.05, got %f", eg.baseRate)
	}
	if eg.personalMul != 2.0 {
		t.Errorf("expected personalMul 2.0, got %f", eg.personalMul)
	}
	if eg.interactMul != 1.5 {
		t.Errorf("expected interactMul 1.5, got %f", eg.interactMul)
	}
	if eg.orgMul != 1.0 {
		t.Errorf("expected orgMul 1.0, got %f", eg.orgMul)
	}
	if eg.regionMul != 0.5 {
		t.Errorf("expected regionMul 0.5, got %f", eg.regionMul)
	}
}

func TestEventGenerator_calculateNPCEventProbability(t *testing.T) {
	eg := NewEventGenerator()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)

	prob := eg.calculateNPCEventProbability(npc, BehaviorIdle)
	if prob <= 0 {
		t.Error("probability should be positive for alive NPC")
	}

	npc.IsAlive = false
	prob = eg.calculateNPCEventProbability(npc, BehaviorIdle)
	if prob != 0 {
		t.Error("probability should be 0 for dead NPC")
	}
	npc.IsAlive = true

	npc.Health = 20
	lowHealthProb := eg.calculateNPCEventProbability(npc, BehaviorIdle)
	npc.Health = 90
	highHealthProb := eg.calculateNPCEventProbability(npc, BehaviorIdle)
	if lowHealthProb <= highHealthProb {
		t.Errorf("low health should have higher probability: %f vs %f", lowHealthProb, highHealthProb)
	}

	sleepProb := eg.calculateNPCEventProbability(npc, BehaviorSleep)
	socialProb := eg.calculateNPCEventProbability(npc, BehaviorSocialize)
	if sleepProb >= socialProb {
		t.Errorf("sleep should have lower probability than socialize: %f vs %f", sleepProb, socialProb)
	}
}

func TestEventGenerator_selectPersonalEventType(t *testing.T) {
	eg := NewEventGenerator()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)

	eventType := eg.selectPersonalEventType(npc, BehaviorWork, rng)
	if eventType.Category() != "personal" {
		t.Errorf("expected personal category, got %s", eventType.Category())
	}

	npc.Profession = ProfNone
	eventType = eg.selectPersonalEventType(npc, BehaviorWork, rng)
	if eventType == GenEventPersonalPromotion {
		t.Error("unemployed NPC should not have promotion event")
	}
}

func TestEventGenerator_selectInteractionEventType(t *testing.T) {
	eg := NewEventGenerator()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)

	eventType := eg.selectInteractionEventType(npc, BehaviorSocialize, rng)
	if eventType.Category() != "interaction" {
		t.Errorf("expected interaction category, got %s", eventType.Category())
	}
}

func TestEventGenerator_GenerateForNPC(t *testing.T) {
	eg := NewEventGenerator()
	eg.SetRates(0.5, 1.0, 1.0, 0.5, 0.3)
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	bs := BehaviorState{CurrentBehavior: BehaviorIdle}

	totalEvents := 0
	for i := 0; i < 100; i++ {
		events := eg.GenerateForNPC(npc, bs, uint32(i), rng)
		totalEvents += len(events)
	}

	if totalEvents == 0 {
		t.Fatal("expected at least some events in 100 iterations with high rate")
	}

	events := eg.GenerateForNPC(npc, bs, 100, rng)
	for _, e := range events {
		if e.EntityID != npc.ID {
			t.Errorf("expected EntityID %d, got %d", npc.ID, e.EntityID)
		}
		if e.Type.Category() != "personal" && e.Type.Category() != "interaction" {
			t.Errorf("unexpected event category: %s", e.Type.Category())
		}
	}
}

func TestEventGenerator_GenerateBatch(t *testing.T) {
	eg := NewEventGenerator()
	rng := rand.New(rand.NewSource(42))

	npcs := make([]*NPCV3, 100)
	for i := 0; i < 100; i++ {
		npcs[i] = GenerateNPCV3(uint64(i+1), rng)
	}

	behaviorStates := make(map[uint64]*BehaviorState)
	for _, npc := range npcs {
		bs := BehaviorState{CurrentBehavior: BehaviorWork}
		behaviorStates[npc.ID] = &bs
	}

	events := eg.GenerateBatch(npcs, behaviorStates, 100, rng)
	if events == nil {
		t.Fatal("GenerateBatch returned nil")
	}

	cats := countEventsByCategory(events)
	if cats["personal"] == 0 && cats["interaction"] == 0 {
		t.Error("expected at least some personal or interaction events")
	}
}

func TestEventGenerator_GenerateOrgEvent(t *testing.T) {
	eg := NewEventGenerator()
	rng := rand.New(rand.NewSource(42))

	eventCount := 0
	for i := 0; i < 1000; i++ {
		evt := eg.GenerateOrgEvent(1, uint32(i), rng, 50)
		if evt != nil {
			eventCount++
			if evt.OrgID != 1 {
				t.Errorf("expected OrgID 1, got %d", evt.OrgID)
			}
			if evt.Type.Category() != "org" {
				t.Errorf("expected org category, got %s", evt.Type.Category())
			}
		}
	}

	if eventCount == 0 {
		t.Error("expected at least some org events in 1000 iterations")
	}
	if eventCount > 800 {
		t.Errorf("too many org events: %d", eventCount)
	}
}

func TestEventGenerator_GenerateRegionEvent(t *testing.T) {
	eg := NewEventGenerator()
	rng := rand.New(rand.NewSource(42))

	eventCount := 0
	for i := 0; i < 1000; i++ {
		evt := eg.GenerateRegionEvent(1, uint32(i), rng, 1000)
		if evt != nil {
			eventCount++
			if evt.RegionID != 1 {
				t.Errorf("expected RegionID 1, got %d", evt.RegionID)
			}
			if evt.Type.Category() != "region" {
				t.Errorf("expected region category, got %s", evt.Type.Category())
			}
		}
	}

	if eventCount == 0 {
		t.Error("expected at least some region events in 1000 iterations")
	}
	if eventCount > 600 {
		t.Errorf("too many region events: %d", eventCount)
	}
}

func TestCountEventsByCategory(t *testing.T) {
	events := []GeneratedEvent{
		{Type: GenEventPersonalIllness},
		{Type: GenEventPersonalShopping},
		{Type: GenEventInteractionConversation},
		{Type: GenEventOrgMeeting},
		{Type: GenEventRegionFestival},
	}

	result := countEventsByCategory(events)
	if result["personal"] != 2 {
		t.Errorf("expected 2 personal events, got %d", result["personal"])
	}
	if result["interaction"] != 1 {
		t.Errorf("expected 1 interaction event, got %d", result["interaction"])
	}
	if result["org"] != 1 {
		t.Errorf("expected 1 org event, got %d", result["org"])
	}
	if result["region"] != 1 {
		t.Errorf("expected 1 region event, got %d", result["region"])
	}
}

func BenchmarkEventGenerator_GenerateForNPC(b *testing.B) {
	eg := NewEventGenerator()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	bs := BehaviorState{CurrentBehavior: BehaviorWork}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eg.GenerateForNPC(npc, bs, uint32(i), rng)
	}
}

func BenchmarkEventGenerator_GenerateBatch(b *testing.B) {
	eg := NewEventGenerator()
	rng := rand.New(rand.NewSource(42))

	npcs := make([]*NPCV3, 100)
	behaviorStates := make(map[uint64]*BehaviorState)
	for i := 0; i < 100; i++ {
		npc := GenerateNPCV3(uint64(i+1), rng)
		npcs[i] = npc
		bs := BehaviorState{CurrentBehavior: BehaviorWork}
		behaviorStates[npc.ID] = &bs
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eg.GenerateBatch(npcs, behaviorStates, uint32(i), rng)
	}
}
