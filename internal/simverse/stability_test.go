package simverse

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"
)

const (
	stabilitySampleInterval = 30 * time.Second
)

type stabilitySample struct {
	timestamp   time.Time
	tick        uint32
	heapInuseMB float64
	heapAllocMB float64
	heapObjects uint64
	goroutines  int
	gcCycles    uint32
	tickAvgNs   float64
	cacheSize   int
	brainCount  int
	focusCount  int
	totalMB     float64
}

func (s stabilitySample) String() string {
	return fmt.Sprintf(
		"tick=%d heap_inuse=%.2fMB heap_alloc=%.2fMB objs=%d goroutines=%d gc=%d tick_avg=%.0fns npc_cache=%d brains=%d focus=%d sim_total=%.2fMB",
		s.tick, s.heapInuseMB, s.heapAllocMB,
		s.heapObjects, s.goroutines, s.gcCycles, s.tickAvgNs,
		s.cacheSize, s.brainCount, s.focusCount, s.totalMB,
	)
}

func TestStability_Full30Min(t *testing.T) {
	if os.Getenv("ENCV_STABILITY_FULL") != "1" {
		t.Skip("Skipping 30-min full stability test. Set ENCV_STABILITY_FULL=1 to enable.")
	}

	durationStr := os.Getenv("ENCV_STABILITY_DURATION")
	duration := 30 * time.Minute
	if durationStr != "" {
		if mins, err := strconv.Atoi(durationStr); err == nil && mins > 0 {
			duration = time.Duration(mins) * time.Minute
		}
	}

	t.Logf("=== SimVerse Full Stability Test (%s) ===", duration)
	t.Logf("Start time: %s", time.Now().Format(time.RFC3339))
	t.Logf("Sampling every %v", stabilitySampleInterval)

	rng := rand.New(rand.NewSource(42))

	world := NewFractalWorld()
	world.SetPerformanceTier(PerfTierBackground)

	for i := 0; i < 100; i++ {
		world.AddFocusNPC(uint64(i), FocusCore)
		world.GetBrain(uint64(i))
	}
	for i := 0; i < 5000; i++ {
		world.GetNPC(uint64(i), rng)
	}

	t.Logf("Pre-warmed: 5000 NPCs in cache, 100 focus NPCs with brains")

	samples := []stabilitySample{}
	deadline := time.Now().Add(duration)
	totalTicks := 0

	startSample := captureStabilitySample(world, 0)
	samples = append(samples, startSample)
	t.Logf("Baseline: %s", startSample)

	tickWindow := make([]int64, 0, 1000)
	nextSample := time.Now().Add(stabilitySampleInterval)
	sampleCount := 1

	for time.Now().Before(deadline) {
		tickStart := time.Now()

		world.Tick(rng)
		totalTicks++

		tickNs := time.Since(tickStart).Nanoseconds()
		tickWindow = append(tickWindow, tickNs)
		if len(tickWindow) > 1000 {
			tickWindow = tickWindow[1:]
		}

		if time.Now().After(nextSample) {
			avgTickNs := int64(0)
			for _, ns := range tickWindow {
				avgTickNs += ns
			}
			if len(tickWindow) > 0 {
				avgTickNs /= int64(len(tickWindow))
			}

			s := captureStabilitySample(world, float64(avgTickNs))
			samples = append(samples, s)
			t.Logf("Sample #%d: %s", sampleCount, s)

			sampleCount++
			nextSample = time.Now().Add(stabilitySampleInterval)
		}
	}

	endSample := captureStabilitySample(world, 0)
	samples = append(samples, endSample)
	t.Logf("Final: %s", endSample)

	t.Log("")
	t.Log("=== Stability Test Results ===")
	t.Logf("Total duration: %s", duration)
	t.Logf("Total ticks: %d", totalTicks)
	t.Logf("Samples taken: %d", len(samples))
	t.Logf("Avg ticks/sec: %.0f", float64(totalTicks)/duration.Seconds())

	if len(samples) >= 2 {
		first := samples[0]
		last := samples[len(samples)-1]

		memGrowth := last.heapInuseMB - first.heapInuseMB
		memGrowthPct := 0.0
		if first.heapInuseMB > 0 {
			memGrowthPct = (memGrowth / first.heapInuseMB) * 100
		}

		t.Logf("")
		t.Logf("HeapInuse: start=%.2fMB end=%.2fMB growth=%.2fMB (%.1f%%)",
			first.heapInuseMB, last.heapInuseMB, memGrowth, memGrowthPct)

		goroutineGrowth := last.goroutines - first.goroutines
		t.Logf("Goroutines: start=%d end=%d delta=%+d",
			first.goroutines, last.goroutines, goroutineGrowth)

		heapObjGrowth := int64(last.heapObjects) - int64(first.heapObjects)
		t.Logf("HeapObjects: start=%d end=%d delta=%+d",
			first.heapObjects, last.heapObjects, heapObjGrowth)

		gcGrowth := last.gcCycles - first.gcCycles
		t.Logf("GC cycles: start=%d end=%d delta=%+d",
			first.gcCycles, last.gcCycles, gcGrowth)

		t.Logf("Sim memory (world stats): start=%.2fMB end=%.2fMB",
			first.totalMB, last.totalMB)

		memThresholdMB := first.heapInuseMB * 0.05
		if memThresholdMB < 1.0 {
			memThresholdMB = 1.0
		}
		if memGrowth > memThresholdMB {
			t.Errorf("⚠️  Memory growth of %.2fMB (%.1f%%) exceeds 5%% threshold (possible leak)",
				memGrowth, memGrowthPct)
		} else {
			t.Log("✅ Memory growth within acceptable range (< 5%)")
		}

		goroutineThreshold := first.goroutines / 5
		if goroutineThreshold < 5 {
			goroutineThreshold = 5
		}
		if absIntStab(goroutineGrowth) > goroutineThreshold {
			t.Errorf("⚠️  Goroutine count changed by %+d (exceeds %d threshold, possible leak)",
				goroutineGrowth, goroutineThreshold)
		} else {
			t.Log("✅ Goroutine count stable")
		}
	}

	t.Log("")
	t.Log("=== Test Complete ===")
	t.Logf("End time: %s", time.Now().Format(time.RFC3339))
}

func captureStabilitySample(world *FractalWorld, avgTickNs float64) stabilitySample {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := world.MemoryStats()

	return stabilitySample{
		timestamp:   time.Now(),
		tick:        world.WorldTick(),
		heapInuseMB: float64(m.HeapInuse) / 1024 / 1024,
		heapAllocMB: float64(m.HeapAlloc) / 1024 / 1024,
		heapObjects: m.HeapObjects,
		goroutines:  runtime.NumGoroutine(),
		gcCycles:    m.NumGC,
		tickAvgNs:   avgTickNs,
		cacheSize:   int(stats["npc_cache_count"]),
		brainCount:  int(stats["brain_cache_count"]),
		focusCount:  int(stats["focus_npc_count"]),
		totalMB:     stats["total_mb"],
	}
}

func absIntStab(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestStability_QuickSanity(t *testing.T) {
	t.Log("Running quick sanity check (10s)")

	rng := rand.New(rand.NewSource(123))
	world := NewFractalWorld()
	world.SetPerformanceTier(PerfTierBackground)

	for i := 0; i < 10; i++ {
		world.AddFocusNPC(uint64(i), FocusCore)
		world.GetBrain(uint64(i))
	}
	for i := 0; i < 1000; i++ {
		world.GetNPC(uint64(i), rng)
	}

	deadline := time.Now().Add(10 * time.Second)
	ticks := 0

	for time.Now().Before(deadline) {
		world.Tick(rng)
		ticks++
	}

	t.Logf("Quick sanity: %d ticks in 10s (%.0f ticks/sec)", ticks, float64(ticks)/10)
	if ticks < 100 {
		t.Errorf("Expected at least 100 ticks in 10s, got %d", ticks)
	} else {
		t.Logf("✅ Performance acceptable")
	}

	stats := world.MemoryStats()
	t.Logf("NPC cache: %.0f entries (%.2f MB)", stats["npc_cache_count"], stats["npc_cache_mb"])
	t.Logf("Brain cache: %.0f entries (%.2f MB)", stats["brain_cache_count"], stats["brain_cache_mb"])
	t.Logf("Cell cache: %.0f entries (%.2f MB)", stats["cell_cache_count"], stats["cell_cache_mb"])
	t.Logf("Total sim memory: %.2f MB", stats["total_mb"])
}
