package simverse

import (
	"math/rand"
	"testing"
)

func TestPersonality_Generate(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	n := 100
	opennessSum := 0
	extroversionSum := 0
	mageHighOpen := 0
	warriorHighExtro := 0

	for i := 0; i < n; i++ {
		species := SpeciesType(i % int(SpeciesMax))
		prof := ProfessionType((i * 3) % int(ProfMax))
		gender := Gender(i % 3)

		p := GeneratePersonality(rng, species, prof, gender)

		for j := 0; j < int(BigFiveMax); j++ {
			if p.BigFive[j] < 5 || p.BigFive[j] > 95 {
				t.Errorf("BigFive trait %d out of range: %d", j, p.BigFive[j])
			}
		}
		for j := 0; j < int(ValueMax); j++ {
			if p.Values[j] < 5 || p.Values[j] > 95 {
				t.Errorf("Value %d out of range: %d", j, p.Values[j])
			}
		}
		for j := 0; j < int(InterestMax); j++ {
			if p.Interests[j] > 95 {
				t.Errorf("Interest %d out of range: %d", j, p.Interests[j])
			}
		}

		opennessSum += int(p.BigFive[TraitOpenness])
		extroversionSum += int(p.BigFive[TraitExtraversion])

		if prof == ProfMage && p.BigFive[TraitOpenness] >= 60 {
			mageHighOpen++
		}
		if prof == ProfWarrior && p.BigFive[TraitExtraversion] >= 60 {
			warriorHighExtro++
		}
	}

	avgOpen := float64(opennessSum) / float64(n)
	avgExtro := float64(extroversionSum) / float64(n)

	t.Logf("Avg Openness: %.1f, Avg Extraversion: %.1f", avgOpen, avgExtro)
	t.Logf("Mages with high openness: %d/%d", mageHighOpen, n/int(ProfMax)+1)
	t.Logf("Warriors with high extroversion: %d/%d", warriorHighExtro, n/int(ProfMax)+1)

	if avgOpen < 30 || avgOpen > 70 {
		t.Errorf("Avg openness suspicious: %.1f", avgOpen)
	}
	if avgExtro < 30 || avgExtro > 70 {
		t.Errorf("Avg extroversion suspicious: %.1f", avgExtro)
	}
}

func TestPersonality_BigFiveMethods(t *testing.T) {
	var bf BigFive
	bf[TraitOpenness] = 80
	bf[TraitConscientious] = 20

	if !bf.IsHigh(TraitOpenness) {
		t.Error("Expected high openness")
	}
	if !bf.IsLow(TraitConscientious) {
		t.Error("Expected low conscientiousness")
	}
	if bf.IsHigh(TraitExtraversion) {
		t.Error("Extraversion should be 0, not high")
	}
	if bf.Get(TraitOpenness) != 80 {
		t.Errorf("Expected 80, got %d", bf.Get(TraitOpenness))
	}
}

func TestPersonality_ValuesTop3(t *testing.T) {
	var v Values
	v[ValueAchievement] = 90
	v[ValuePower] = 85
	v[ValueHedonism] = 80
	v[ValueSecurity] = 30

	top := v.Top3()
	if len(top) != 3 {
		t.Errorf("Expected 3, got %d", len(top))
	}
	if top[0] != ValueAchievement {
		t.Errorf("Expected top to be Achievement, got %s", top[0])
	}
	if top[1] != ValuePower {
		t.Errorf("Expected second to be Power, got %s", top[1])
	}
	if top[2] != ValueHedonism {
		t.Errorf("Expected third to be Hedonism, got %s", top[2])
	}
}

func TestPersonality_InterestsTopN(t *testing.T) {
	var in Interests
	in[InterestReading] = 95
	in[InterestArt] = 85
	in[InterestMusic] = 75
	in[InterestSports] = 30

	top := in.TopN(3)
	if len(top) != 3 {
		t.Errorf("Expected 3, got %d", len(top))
	}
	if top[0] != InterestReading {
		t.Errorf("Expected top to be Reading, got %s", top[0])
	}
	if !in.Has(InterestReading) {
		t.Error("Should have reading interest")
	}
	if in.Has(InterestSports) {
		t.Error("Should not have sports interest (30 < 40)")
	}
}

func TestMemory_ShortTerm(t *testing.T) {
	stm := NewShortTermMemory()

	if stm.Count() != 0 {
		t.Errorf("Expected 0, got %d", stm.Count())
	}

	for i := 0; i < 10; i++ {
		stm.Add(Memory{
			ID:          uint32(i),
			Type:        MemInteraction,
			Importance:  MemImpModerate,
			TargetID:    uint64(100 + i),
			CreatedTick: uint32(i * 10),
		})
	}

	if stm.Count() != 10 {
		t.Errorf("Expected 10, got %d", stm.Count())
	}

	items := stm.Items()
	if len(items) != 10 {
		t.Errorf("Expected 10 items, got %d", len(items))
	}
	if items[0].ID != 0 {
		t.Errorf("Expected first ID 0, got %d", items[0].ID)
	}

	for i := 10; i < 30; i++ {
		stm.Add(Memory{
			ID:          uint32(i),
			Type:        MemFact,
			Importance:  MemImpMinor,
			TargetID:    uint64(100 + i),
			CreatedTick: uint32(i * 10),
		})
	}

	if stm.Count() != ShortTermMemoryCapacity {
		t.Errorf("Expected capacity %d, got %d", ShortTermMemoryCapacity, stm.Count())
	}

	items = stm.Items()
	if items[0].ID != 10 {
		t.Errorf("After overflow, expected first ID 10, got %d", items[0].ID)
	}

	interactions := stm.ByType(MemInteraction)
	if len(interactions) != 0 {
		t.Errorf("Expected 0 interaction mems after overflow, got %d", len(interactions))
	}

	facts := stm.ByType(MemFact)
	if len(facts) != ShortTermMemoryCapacity {
		t.Errorf("Expected all fact mems, got %d", len(facts))
	}
}

func TestMemory_LongTerm(t *testing.T) {
	ltm := NewLongTermMemory()

	ltm.Add(Memory{
		ID:          1,
		Type:        MemAchievement,
		Importance:  MemImpCritical,
		CreatedTick: 100,
	})
	ltm.Add(Memory{
		ID:          2,
		Type:        MemInteraction,
		Importance:  MemImpTrivial,
		CreatedTick: 200,
	})

	if ltm.Count() != 2 {
		t.Errorf("Expected 2, got %d", ltm.Count())
	}

	ltm.Decay(1000, 0)

	items := ltm.Items()
	for _, m := range items {
		if m.Importance == MemImpCritical && m.Strength < 0.5 {
			t.Errorf("Critical memory shouldn't decay much: strength=%.2f", m.Strength)
		}
		t.Logf("Mem %d (imp=%d): strength=%.3f", m.ID, m.Importance, m.Strength)
	}
}

func TestMemory_System(t *testing.T) {
	ms := NewMemorySystem()

	ms.Record(Memory{
		ID:          1,
		Type:        MemInteraction,
		Importance:  MemImpTrivial,
		CreatedTick: 10,
	})
	ms.Record(Memory{
		ID:          2,
		Type:        MemAchievement,
		Importance:  MemImpMajor,
		CreatedTick: 20,
	})
	ms.Record(Memory{
		ID:          3,
		Type:        MemTrauma,
		Importance:  MemImpCritical,
		CreatedTick: 30,
	})

	if ms.ShortTerm.Count() != 3 {
		t.Errorf("Expected 3 short-term, got %d", ms.ShortTerm.Count())
	}

	longTermCount := ms.LongTerm.Count()
	t.Logf("Long-term memories: %d (should be >= 2 for moderate+)", longTermCount)
	if longTermCount < 2 {
		t.Errorf("Expected at least 2 long-term (moderate+), got %d", longTermCount)
	}

	total := ms.TotalCount()
	t.Logf("Total memories: %d", total)
}

func TestPersonality_Size(t *testing.T) {
	size := PersonalitySize()
	t.Logf("Personality size: %.0f bytes (big5=%d + values=%d + interests=%d)",
		size, BigFiveMax, ValueMax, InterestMax)

	expected := float64(BigFiveMax + ValueMax + InterestMax)
	if size != expected {
		t.Errorf("Expected %.0f, got %.0f", expected, size)
	}

	t.Logf("ShortTermMemory size: %.0f bytes (capacity=%d)",
		ShortTermMemorySize(), ShortTermMemoryCapacity)
}
