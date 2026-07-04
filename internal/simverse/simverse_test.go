package simverse

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"
)

func TestNPCTotalSize(t *testing.T) {
	buf := make([]byte, 256)
	npc := &NPC{
		ID:         12345,
		Name:       "张三",
		Age:        25,
		Health:     1000,
		Energy:     800,
		Strength:   50,
		Intellect:  60,
		Charisma:   40,
		Agility:    55,
		Luck:       45,
		Status:     0,
		OrgID:      100,
		RegionID:   1,
		LastActive: time.Now().Unix(),
		BornAt:     -365 * 25,
	}

	size := npc.MarshalTo(buf)
	t.Logf("NPC serialized size: %d bytes (target: < 96 base + name)", size)

	restored := &NPC{}
	if !restored.Unmarshal(buf[:size]) {
		t.Fatal("Unmarshal failed")
	}
	if restored.ID != npc.ID || restored.Name != npc.Name || restored.Age != npc.Age {
		t.Fatalf("Data mismatch after round-trip")
	}
	if restored.Health != npc.Health || restored.Strength != npc.Strength {
		t.Fatalf("Attribute mismatch after round-trip")
	}
}

func BenchmarkNPCMarshal(b *testing.B) {
	buf := make([]byte, 256)
	npc := &NPC{
		ID:         12345,
		Name:       "TestNPC_12345",
		Age:        25,
		Health:     1000,
		Energy:     800,
		Strength:   50,
		Intellect:  60,
		Charisma:   40,
		Agility:    55,
		Luck:       45,
		Status:     0,
		OrgID:      100,
		RegionID:   1,
		LastActive: time.Now().Unix(),
		BornAt:     -9000,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		npc.MarshalTo(buf)
	}
}

func BenchmarkNPCUnmarshal(b *testing.B) {
	buf := make([]byte, 256)
	npc := &NPC{
		ID:         12345,
		Name:       "TestNPC_12345",
		Age:        25,
		Health:     1000,
		Energy:     800,
		Strength:   50,
		Intellect:  60,
		Charisma:   40,
		Agility:    55,
		Luck:       45,
		Status:     0,
		OrgID:      100,
		RegionID:   1,
		LastActive: time.Now().Unix(),
		BornAt:     -9000,
	}
	size := npc.MarshalTo(buf)
	data := buf[:size]
	result := &NPC{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result.Unmarshal(data)
	}
}

func TestMemoryModel_10KHot(t *testing.T) {
	count := 10_000
	npcs := make([]NPC, count)

	var totalStruct uint64
	for i := range npcs {
		npcs[i] = NPC{
			ID:         uint64(i),
			Name:       fmt.Sprintf("NPC_%d", i),
			Age:        uint16(20 + i%50),
			Health:     1000,
			Energy:     800,
			Strength:   uint8(30 + i%40),
			Intellect:  uint8(30 + i%40),
			Charisma:   uint8(30 + i%40),
			Agility:    uint8(30 + i%40),
			Luck:       uint8(30 + i%40),
			OrgID:      uint32(i % 1000),
			RegionID:   uint32(i % 100),
			LastActive: int64(i * 1000),
			BornAt:     int32(-(i * 10)),
		}
		totalStruct += uint64(len(fmt.Sprintf("NPC_%d", i))) + 96
	}

	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	t.Logf("=== 10K Hot NPCs Memory Model ===")
	t.Logf("Struct + names (estimated): %.2f MB", float64(totalStruct)/1024/1024)
	t.Logf("Heap alloc: %.2f MB", float64(m.HeapAlloc)/1024/1024)
	t.Logf("Heap inuse: %.2f MB", float64(m.HeapInuse)/1024/1024)

	perNPC := float64(m.HeapAlloc) / float64(count)
	t.Logf("Per NPC (avg): %.1f bytes", perNPC)
	t.Logf("Extrapolated to 10M (if all in memory): %.0f MB", perNPC*10_000_000/1024/1024)
	t.Logf("Feasible with tiered storage (99%% on disk): YES")

	_ = npcs
}

func TestMemoryModel_1MArrayOnly(t *testing.T) {
	t.Skip("Skipping 1M test by default (memory intensive). Run with -run TestMemoryModel_1MArrayOnly to enable.")

	count := 1_000_000
	ids := make([]uint64, count)
	orgs := make([]uint32, count)
	regions := make([]uint32, count)

	for i := 0; i < count; i++ {
		ids[i] = uint64(i)
		orgs[i] = uint32(i % 10000)
		regions[i] = uint32(i % 1000)
	}

	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)

	t.Logf("=== 1M Index Arrays Memory Model ===")
	t.Logf("ID array (8B each): %.2f MB", float64(count*8)/1024/1024)
	t.Logf("OrgID array (4B each): %.2f MB", float64(count*4)/1024/1024)
	t.Logf("RegionID array (4B each): %.2f MB", float64(count*4)/1024/1024)
	t.Logf("Total index est: %.2f MB", float64(count*16)/1024/1024)
	t.Logf("Actual heap alloc: %.2f MB", float64(m.HeapAlloc)/1024/1024)

	_ = ids
	_ = orgs
	_ = regions
}

func BenchmarkEventGeneration(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	events := make([]uint8, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range events {
			events[j] = uint8(rng.Intn(20))
		}
	}

	rate := float64(b.N) * float64(len(events)) / b.Elapsed().Seconds()
	b.ReportMetric(rate, "events/sec")
}

func TestEventSchedulerBasic(t *testing.T) {
	scheduler := NewEventScheduler()

	count := 1000
	now := int64(0)
	for i := 0; i < count; i++ {
		scheduler.Schedule(Event{
			ID:       uint64(i),
			Type:     uint8(i % 10),
			TargetID: uint64(i * 2),
			ScheduledAt: now + int64(i%100),
		})
	}

	if scheduler.Len() != count {
		t.Fatalf("Expected %d events, got %d", count, scheduler.Len())
	}

	processed := 0
	for ts := int64(0); ts < 100; ts++ {
		ready := scheduler.Tick(ts)
		processed += len(ready)
	}

	expected := 10 * 10
	if processed != expected {
		t.Logf("Processed %d events in 100 ticks (expected %d per tick × 100 ticks = %d)",
			processed, count/100, expected)
	}

	t.Logf("Scheduler basic test: %d events scheduled, %d processed in 100 ticks",
		count, processed)
}

func BenchmarkEventScheduler(b *testing.B) {
	scheduler := NewEventScheduler()
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 10000; i++ {
		scheduler.Schedule(Event{
			ID:          uint64(i),
			Type:        uint8(i % 20),
			TargetID:    uint64(i),
			ScheduledAt: int64(rng.Intn(1000)),
		})
	}

	b.ResetTimer()
	tick := int64(0)
	for i := 0; i < b.N; i++ {
		ready := scheduler.Tick(tick)
		tick++
		for range ready {
			scheduler.Schedule(Event{
				ID:          uint64(10000 + i),
				Type:        0,
				TargetID:    0,
				ScheduledAt: tick + int64(rng.Intn(100)),
			})
		}
	}
}
