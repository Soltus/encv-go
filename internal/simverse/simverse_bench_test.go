package simverse

import (
	"math/rand"
	"testing"
)

func BenchmarkNPCV3_Generate(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateNPCV3(uint64(i), rng)
	}
}

func BenchmarkNPCV3_CatchUp_1Tick(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	npc.LastUpdateTick = 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		npc.CatchUp(uint32(i+1), rng)
	}
}

func BenchmarkNPCV3_CatchUp_1000Ticks(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	npc.LastUpdateTick = 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		npc.LastUpdateTick = 0
		npc.CatchUp(1000, rng)
	}
}

func BenchmarkNPCV3_Marshal(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	buf := make([]byte, 256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		npc.MarshalToV3(buf)
	}
}

func BenchmarkNPCV3_Unmarshal(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	buf := make([]byte, 256)
	n := npc.MarshalToV3(buf)
	data := buf[:n]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var npc2 NPCV3
		npc2.UnmarshalV3(data)
	}
}

func BenchmarkNPCV3_SerializeRoundtrip(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	npc := GenerateNPCV3(1, rng)
	buf := make([]byte, 256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n := npc.MarshalToV3(buf)
		var npc2 NPCV3
		npc2.UnmarshalV3(buf[:n])
	}
}

func BenchmarkBrain_CatchUp(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	brain := NewBrain(1)
	npc := GenerateNPCV3(1, rng)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		brain.CatchUp(uint32(i), rng, npc)
	}
}

func BenchmarkBrain_GetActiveCells_100(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	brain := NewBrain(1)
	npc := GenerateNPCV3(1, rng)
	brain.CatchUp(1000, rng, npc)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = brain.GetActiveCells(BrainRegionFrontal, 100, npc.Age, rng)
	}
}

func BenchmarkCache_PutGet_10K(b *testing.B) {
	cache := NewEntityCache[*NPCV3](10000)
	rng := rand.New(rand.NewSource(42))
	npcs := make([]*NPCV3, 10000)
	for i := 0; i < 10000; i++ {
		npcs[i] = GenerateNPCV3(uint64(i), rng)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := uint64(i % 10000)
		cache.Put(idx, npcs[idx])
		_, _ = cache.Get(idx)
	}
}

func BenchmarkFractalWorld_Tick_100NPCs(b *testing.B) {
	world := NewFractalWorld()
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 100; i++ {
		_ = world.GetNPC(uint64(i), rng)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		world.Tick(rng)
	}
}

func BenchmarkFractalWorld_Tick_1000NPCs(b *testing.B) {
	world := NewFractalWorld()
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 1000; i++ {
		_ = world.GetNPC(uint64(i), rng)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		world.Tick(rng)
	}
}

func BenchmarkFractalWorld_Tick_10000NPCs(b *testing.B) {
	world := NewFractalWorld()
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 10000; i++ {
		_ = world.GetNPC(uint64(i), rng)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		world.Tick(rng)
	}
}

func BenchmarkFractalWorld_GetNPC(b *testing.B) {
	world := NewFractalWorld()
	rng := rand.New(rand.NewSource(42))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = world.GetNPC(uint64(i%10000), rng)
	}
}
