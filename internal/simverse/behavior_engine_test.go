package simverse

import (
	"math/rand"
	"testing"
)

func TestBehaviorTypeString(t *testing.T) {
	tests := []struct {
		bt       BehaviorType
		expected string
	}{
		{BehaviorIdle, "idle"},
		{BehaviorWork, "work"},
		{BehaviorRest, "rest"},
		{BehaviorEat, "eat"},
		{BehaviorSleep, "sleep"},
		{BehaviorSocialize, "socialize"},
		{BehaviorExplore, "explore"},
		{BehaviorTrade, "trade"},
		{BehaviorMax, "unknown"},
	}

	for _, tt := range tests {
		if got := tt.bt.String(); got != tt.expected {
			t.Errorf("BehaviorType(%d).String() = %q, want %q", tt.bt, got, tt.expected)
		}
	}
}

func TestBehaviorTypeCN(t *testing.T) {
	tests := []struct {
		bt       BehaviorType
		expected string
	}{
		{BehaviorIdle, "空闲"},
		{BehaviorWork, "工作"},
		{BehaviorRest, "休息"},
		{BehaviorEat, "进食"},
		{BehaviorSleep, "睡眠"},
		{BehaviorSocialize, "社交"},
		{BehaviorExplore, "探索"},
		{BehaviorTrade, "交易"},
		{BehaviorMax, "未知"},
	}

	for _, tt := range tests {
		if got := tt.bt.CN(); got != tt.expected {
			t.Errorf("BehaviorType(%d).CN() = %q, want %q", tt.bt, got, tt.expected)
		}
	}
}

func TestNewBehaviorEngine(t *testing.T) {
	be := NewBehaviorEngine()
	if be == nil {
		t.Fatal("NewBehaviorEngine() returned nil")
	}
	if be.tickRate != 1 {
		t.Errorf("expected tickRate=1, got %d", be.tickRate)
	}
}

func TestBehaviorEngine_InitNPC(t *testing.T) {
	be := NewBehaviorEngine()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)

	state := be.InitNPC(npc, rng)

	if state.CurrentBehavior != BehaviorIdle {
		t.Errorf("expected initial behavior Idle, got %v", state.CurrentBehavior)
	}
	if state.BehaviorStartTick != 0 {
		t.Errorf("expected start tick 0, got %d", state.BehaviorStartTick)
	}
	if state.Needs.Hunger > 50 {
		t.Errorf("initial hunger too high: %d", state.Needs.Hunger)
	}
	if state.Needs.Energy < 60 {
		t.Errorf("initial energy too low: %d", state.Needs.Energy)
	}
}

func TestBehaviorEngine_TickNeeds(t *testing.T) {
	be := NewBehaviorEngine()
	state := &BehaviorState{
		CurrentBehavior: BehaviorIdle,
		Needs: NeedSystem{
			Hunger:      50,
			Energy:      80,
			Social:      50,
			Achievement: 50,
		},
	}

	be.TickNeeds(state, 10)

	if state.Needs.Hunger <= 50 {
		t.Errorf("hunger should increase during idle, got %d", state.Needs.Hunger)
	}
	if state.Needs.Energy >= 80 {
		t.Errorf("energy should decrease during idle, got %d", state.Needs.Energy)
	}
	if state.Needs.Social <= 50 {
		t.Errorf("social should increase during idle, got %d", state.Needs.Social)
	}
	if state.Needs.Achievement <= 50 {
		t.Errorf("achievement should increase during idle, got %d", state.Needs.Achievement)
	}
}

func TestBehaviorEngine_TickNeeds_Sleep(t *testing.T) {
	be := NewBehaviorEngine()
	state := &BehaviorState{
		CurrentBehavior: BehaviorSleep,
		Needs: NeedSystem{
			Hunger: 50,
			Energy: 20,
			Social: 50,
		},
	}

	be.TickNeeds(state, 50)

	if state.Needs.Energy <= 20 {
		t.Errorf("energy should increase during sleep, got %d", state.Needs.Energy)
	}
	if state.Needs.Energy > 100 {
		t.Errorf("energy should not exceed 100, got %d", state.Needs.Energy)
	}
}

func TestBehaviorEngine_TickNeeds_Eat(t *testing.T) {
	be := NewBehaviorEngine()
	state := &BehaviorState{
		CurrentBehavior: BehaviorEat,
		Needs: NeedSystem{
			Hunger: 200,
			Energy: 80,
		},
	}

	be.TickNeeds(state, 10)

	if state.Needs.Hunger >= 200 {
		t.Errorf("hunger should decrease during eating, got %d", state.Needs.Hunger)
	}
}

func TestBehaviorEngine_DecideNextAction(t *testing.T) {
	be := NewBehaviorEngine()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)

	state := &BehaviorState{
		Needs: NeedSystem{
			Hunger:      250,
			Energy:      80,
			Social:      50,
			Achievement: 50,
		},
	}

	behavior := be.DecideNextAction(npc, state, rng)

	if behavior != BehaviorEat {
		t.Errorf("expected BehaviorEat when very hungry, got %v", behavior)
	}
}

func TestBehaviorEngine_DecideNextAction_Sleepy(t *testing.T) {
	be := NewBehaviorEngine()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)

	state := &BehaviorState{
		Needs: NeedSystem{
			Hunger:      50,
			Energy:      10,
			Social:      50,
			Achievement: 50,
		},
	}

	behavior := be.DecideNextAction(npc, state, rng)

	if behavior != BehaviorSleep {
		t.Errorf("expected BehaviorSleep when very tired, got %v", behavior)
	}
}

func TestBehaviorEngine_GetBehaviorDuration(t *testing.T) {
	be := NewBehaviorEngine()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)

	durations := make(map[BehaviorType]bool)
	for i := 0; i < int(BehaviorMax); i++ {
		bt := BehaviorType(i)
		dur := be.GetBehaviorDuration(bt, npc, rng)
		if dur == 0 {
			t.Errorf("duration for %v should not be 0", bt)
		}
		durations[bt] = true
	}

	if len(durations) != int(BehaviorMax) {
		t.Errorf("expected %d behavior types, got %d", BehaviorMax, len(durations))
	}
}

func TestBehaviorEngine_Tick(t *testing.T) {
	be := NewBehaviorEngine()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	state := be.InitNPC(npc, rng)

	state.BehaviorDuration = 10
	state.BehaviorStartTick = 0
	state.CurrentBehavior = BehaviorIdle

	changed := be.Tick(npc, &state, 5, rng)
	if changed {
		t.Error("behavior should not change before duration ends")
	}

	changed = be.Tick(npc, &state, 15, rng)
	if !changed {
		t.Error("behavior should change after duration ends")
	}
}

func TestBehaviorEngine_ExecuteBehavior_Work(t *testing.T) {
	be := NewBehaviorEngine()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	npc.Skills[0] = 10

	state := &BehaviorState{
		CurrentBehavior: BehaviorWork,
		Needs: NeedSystem{
			Hunger:      50,
			Energy:      80,
			Social:      50,
			Achievement: 80,
		},
	}

	initialExp := npc.Experience
	for i := 0; i < 1000; i++ {
		be.ExecuteBehavior(npc, state, 1, rng)
	}

	if npc.Experience <= initialExp {
		t.Errorf("expected experience to increase during work, got %d (was %d)", npc.Experience, initialExp)
	}
}

func TestNeedSystem_Clone(t *testing.T) {
	original := NeedSystem{
		Hunger:      100,
		Energy:      80,
		Social:      60,
		Achievement: 40,
	}

	cloned := original.Clone()

	if cloned.Hunger != original.Hunger {
		t.Errorf("cloned hunger mismatch: %d vs %d", cloned.Hunger, original.Hunger)
	}
	if cloned.Energy != original.Energy {
		t.Errorf("cloned energy mismatch: %d vs %d", cloned.Energy, original.Energy)
	}

	cloned.Hunger = 200
	if original.Hunger == cloned.Hunger {
		t.Error("clone should be independent of original")
	}
}

func BenchmarkBehaviorEngine_Tick(b *testing.B) {
	be := NewBehaviorEngine()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	state := be.InitNPC(npc, rng)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		be.Tick(npc, &state, uint32(i), rng)
	}
}

func BenchmarkBehaviorEngine_DecideNextAction(b *testing.B) {
	be := NewBehaviorEngine()
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	state := be.InitNPC(npc, rng)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		be.DecideNextAction(npc, &state, rng)
	}
}

func BenchmarkBehaviorEngine_TickNeeds(b *testing.B) {
	be := NewBehaviorEngine()
	state := &BehaviorState{
		CurrentBehavior: BehaviorIdle,
		Needs: NeedSystem{
			Hunger:      50,
			Energy:      80,
			Social:      50,
			Achievement: 50,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		be.TickNeeds(state, 1)
	}
}
