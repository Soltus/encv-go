package simverse

import (
	"encoding/binary"
	"math/rand"
)

func (n *NPCV3) UnmarshalV3(buf []byte) bool {
	if len(buf) < NPCV3BaseSize {
		return false
	}

	off := 0
	n.ID = binary.LittleEndian.Uint64(buf[off : off+8])
	off += 8
	n.Species = SpeciesType(buf[off])
	off++
	n.Gender = Gender(buf[off])
	off++
	n.GenderIdentity = GenderIdentity(buf[off])
	off++
	n.SexualOrient = SexualOrientation(buf[off])
	off++
	n.Profession = ProfessionType(binary.LittleEndian.Uint16(buf[off : off+2]))
	off += 2
	n.Level = binary.LittleEndian.Uint16(buf[off : off+2])
	off += 2
	n.Age = binary.LittleEndian.Uint16(buf[off : off+2])
	off += 2
	n.Health = binary.LittleEndian.Uint16(buf[off : off+2])
	off += 2
	n.MaxHealth = binary.LittleEndian.Uint16(buf[off : off+2])
	off += 2
	n.Energy = binary.LittleEndian.Uint16(buf[off : off+2])
	off += 2
	n.MaxEnergy = binary.LittleEndian.Uint16(buf[off : off+2])
	off += 2
	n.Mana = binary.LittleEndian.Uint16(buf[off : off+2])
	off += 2
	n.MaxMana = binary.LittleEndian.Uint16(buf[off : off+2])
	off += 2
	n.Mood = int8(buf[off])
	off++
	n.Satisfaction = int8(buf[off])
	off++

	for i := 0; i < int(SkillMax); i++ {
		n.Skills[i] = buf[off]
		off++
	}

	for i := 0; i < int(ResMax); i++ {
		n.Inventory[i] = binary.LittleEndian.Uint32(buf[off : off+4])
		off += 4
	}

	for i := 0; i < int(ResMax); i++ {
		n.Bank[i] = binary.LittleEndian.Uint32(buf[off : off+4])
		off += 4
	}

	n.OrgID = binary.LittleEndian.Uint32(buf[off : off+4])
	off += 4
	n.RegionID = binary.LittleEndian.Uint32(buf[off : off+4])
	off += 4
	n.HomeRegionID = binary.LittleEndian.Uint32(buf[off : off+4])
	off += 4
	n.BornAt = int32(binary.LittleEndian.Uint32(buf[off : off+4]))
	off += 4
	n.DiedAt = int32(binary.LittleEndian.Uint32(buf[off : off+4]))
	off += 4
	n.IsAlive = buf[off] == 1
	off++
	n.LastUpdateTick = binary.LittleEndian.Uint32(buf[off : off+4])
	off += 4
	n.Experience = binary.LittleEndian.Uint32(buf[off : off+4])
	off += 4
	n.WealthTier = buf[off]
	off++
	n.SocialTier = buf[off]
	off++
	n.CareerStage = buf[off]
	off++
	n.LifeStage = LifeStage(buf[off])
	off++
	n.NumChildren = buf[off]
	off++
	n.NumMarriages = buf[off]
	off++

	for i := 0; i < 8; i++ {
		n.FactionRep[i] = int16(binary.LittleEndian.Uint16(buf[off : off+2]))
		off += 2
	}

	n.LifeEvents = binary.LittleEndian.Uint32(buf[off : off+4])
	off += 4

	if off >= len(buf) {
		return false
	}
	nameLen := int(buf[off])
	off++
	if off+nameLen > len(buf) || nameLen > 50 {
		return false
	}
	n.Name = string(buf[off : off+nameLen])
	return true
}

func GenerateNPCV3(id uint64, rng *rand.Rand) *NPCV3 {
	species := SpeciesType(id % uint64(SpeciesMax))

	genderRoll := rng.Float64()
	var gender Gender
	switch {
	case genderRoll < 0.49:
		gender = GenderMale
	case genderRoll < 0.98:
		gender = GenderFemale
	default:
		gender = GenderNonBinary
	}

	var genderIdentity GenderIdentity
	switch gender {
	case GenderMale:
		if rng.Float64() < 0.01 {
			genderIdentity = GenderIdentTransMale
		} else {
			genderIdentity = GenderIdentCisMale
		}
	case GenderFemale:
		if rng.Float64() < 0.01 {
			genderIdentity = GenderIdentTransFemale
		} else {
			genderIdentity = GenderIdentCisFemale
		}
	default:
		genderIdentity = GenderIdentNonBinary
	}

	var sexualOrient SexualOrientation
	roll := rng.Float64()
	switch {
	case roll < 0.88:
		sexualOrient = SexOrientHetero
	case roll < 0.94:
		sexualOrient = SexOrientBi
	case roll < 0.98:
		sexualOrient = SexOrientHomo
	default:
		sexualOrient = SexOrientAsexual
	}

	stageAges := [LifeStageMax]float64{
		0.02, 0.12, 0.15, 0.30, 0.25, 0.12, 0.04,
	}
	cumulative := 0.0
	ageRoll := rng.Float64()
	var stage LifeStage
	var ageInStage float64
	for s := LifeStage(0); s < LifeStageMax; s++ {
		cumulative += stageAges[s]
		if ageRoll < cumulative {
			stage = s
			ageInStage = (ageRoll - (cumulative - stageAges[s])) / stageAges[s]
			break
		}
	}

	baseAges := [SpeciesMax][LifeStageMax]uint16{
		SpeciesHuman:     {3, 12, 18, 35, 55, 75, 100},
		SpeciesElf:       {10, 50, 100, 200, 400, 600, 1000},
		SpeciesDwarf:     {5, 20, 40, 80, 150, 250, 400},
		SpeciesOrc:       {2, 8, 14, 25, 40, 55, 80},
		SpeciesBeastman:  {2, 10, 16, 28, 45, 65, 90},
		SpeciesDragonkin: {20, 80, 150, 300, 600, 1000, 2000},
		SpeciesFey:       {5, 25, 50, 100, 200, 300, 500},
		SpeciesUndead:    {0, 0, 0, 0, 0, 0, 0},
	}
	thresholds := baseAges[species]
	var age uint16
	if stage == 0 {
		age = uint16(float64(thresholds[0]) * ageInStage)
	} else {
		prev := thresholds[stage-1]
		span := thresholds[stage] - prev
		age = prev + uint16(float64(span)*ageInStage)
	}

	prof := chooseStartingProfessionForGen(species, age, stage, rng)

	baseStat := uint16(50 + rng.Intn(80))
	maxHealth := baseStat + uint16(rng.Intn(200))
	maxEnergy := uint16(400 + rng.Intn(400))
	maxMana := uint16(100 + rng.Intn(400))

	skillBase := uint8(10 + rng.Intn(20))
	var skills Skills
	rates := stageSkillRate(stage)
	for i := 0; i < int(SkillMax); i++ {
		skillMax := int(rates[i] * 2)
		if skillMax <= 0 {
			skills[i] = skillBase / 2
		} else {
			skills[i] = skillBase + uint8(rng.Intn(skillMax))
		}
	}

	wealthTier := uint8(rng.Intn(5))
	socialTier := uint8(rng.Intn(4))
	if species == SpeciesElf {
		socialTier = uint8(minInt(int(socialTier)+1, 3))
	}
	if species == SpeciesOrc {
		wealthTier = uint8(max(0, int(wealthTier)-1))
	}

	level := uint16(0)
	if stage >= LifeStageAdolescent {
		level = uint16(uint16(stage)*10 + uint16(rng.Intn(15)))
	}

	careerStage := uint8(0)
	if stage >= LifeStageYoungAdult {
		careerStage = uint8(stage - LifeStageYoungAdult + 1)
	}

	var name string
	switch species {
	case SpeciesHuman:
		name = GenerateChineseName(rng, gender)
	default:
		name = GenerateChineseName(rng, gender)
	}

	var inventory, bank Resources
	startGold := uint32(10+rng.Intn(50)) * uint32(wealthTier+1)
	bank[ResGold] = startGold
	inventory[ResFood] = uint32(10 + rng.Intn(20))

	personality := GeneratePersonality(rng, species, prof, gender)

	return &NPCV3{
		ID:             id,
		Name:           name,
		Species:        species,
		Gender:         gender,
		GenderIdentity: genderIdentity,
		SexualOrient:   sexualOrient,
		Profession:     prof,
		Level:          level,
		Age:            age,
		Health:         maxHealth,
		MaxHealth:      maxHealth,
		Energy:         maxEnergy,
		MaxEnergy:      maxEnergy,
		Mana:           maxMana,
		MaxMana:        maxMana,
		Mood:           int8(rng.Intn(60) - 20),
		Satisfaction:   int8(rng.Intn(60) - 20),
		Skills:         skills,
		Inventory:      inventory,
		Bank:           bank,
		OrgID:          uint32(prof)%OrgMax + 1,
		RegionID:       uint32(id % 100),
		HomeRegionID:   uint32(id % 100),
		BornAt:         0,
		DiedAt:         0,
		IsAlive:        true,
		LastUpdateTick: 0,
		Experience:     uint32(level) * 100,
		WealthTier:     wealthTier,
		SocialTier:     socialTier,
		CareerStage:    careerStage,
		LifeStage:      stage,
		NumChildren:    0,
		NumMarriages:   0,
		FactionRep:     [8]int16{},
		LifeEvents:     uint32(stage),
		Personality:    personality,
	}
}

func chooseStartingProfessionForGen(species SpeciesType, age uint16, stage LifeStage, rng *rand.Rand) ProfessionType {
	if stage < LifeStageAdolescent {
		return ProfNone
	}

	weights := make([]float64, ProfMax)
	for i := ProfessionType(1); i < ProfMax; i++ {
		weights[i] = 1.0
	}

	weights[ProfFarmer] *= 3.0
	weights[ProfMiner] *= 1.5
	weights[ProfWoodcutter] *= 1.5
	weights[ProfMerchant] *= 1.2
	weights[ProfWarrior] *= 1.0
	weights[ProfMage] *= 0.5
	weights[ProfPriest] *= 0.8
	weights[ProfNoble] *= 0.2
	weights[ProfSlave] *= 0.1
	weights[ProfHealer] *= 0.7
	weights[ProfRogue] *= 0.5
	weights[ProfRanger] *= 0.8
	weights[ProfEntertainer] *= 0.6
	weights[ProfCook] *= 0.7
	weights[ProfAlchemist] *= 0.4
	weights[ProfScribe] *= 0.5
	weights[ProfBlacksmith] *= 0.9
	weights[ProfTailor] *= 0.8
	weights[ProfBeggar] *= 0.15

	switch species {
	case SpeciesElf:
		weights[ProfMage] *= 3
		weights[ProfRanger] *= 2
		weights[ProfPriest] *= 1.5
		weights[ProfWarrior] *= 0.5
	case SpeciesDwarf:
		weights[ProfMiner] *= 3
		weights[ProfBlacksmith] *= 2.5
		weights[ProfWarrior] *= 1.5
		weights[ProfMage] *= 0.3
	case SpeciesOrc:
		weights[ProfWarrior] *= 3
		weights[ProfRanger] *= 2
		weights[ProfFarmer] *= 0.5
		weights[ProfMage] *= 0.2
	case SpeciesDragonkin:
		weights[ProfWarrior] *= 2
		weights[ProfMage] *= 2
		weights[ProfNoble] *= 3
		weights[ProfSlave] = 0
	case SpeciesUndead:
		weights[ProfWarrior] *= 2
		weights[ProfMage] *= 1.5
		weights[ProfPriest] = 0
	}

	total := 0.0
	for _, w := range weights {
		total += w
	}

	r := rng.Float64() * total
	cumulative := 0.0
	for i, w := range weights {
		cumulative += w
		if r < cumulative {
			return ProfessionType(i)
		}
	}

	return ProfFarmer
}

func (n *NPCV3) MemorySize() float64 {
	return float64(NPCV3BaseSize) + float64(len(n.Name))
}

func (b *Brain) CatchUp(currentTick uint32, rng *rand.Rand, npc *NPCV3) {
	if npc == nil {
		return
	}

	if b.Stats.TotalCells == 0 {
		b.Stats.TotalCells = 86_000_000_000
	}

	stage := npc.LifeStage
	baseActivity := stageBrainActivity(stage)

	for region := BrainRegion(0); region < BrainRegionMax; region++ {
		stat := &b.Stats.RegionStats[region]
		if stat.CellCount == 0 {
			stat.CellCount = uint32(float64(b.Stats.TotalCells) * regionCellRatio(region))
			stat.ActivityAvg = uint16(baseActivity * 1000)
			stat.WeightAvg = uint16(200 + rng.Intn(300))
		}

		delta := int(baseActivity*50*rng.Float64()) - 25
		if delta > 0 {
			if stat.ActivityAvg+uint16(delta) > 1000 {
				stat.ActivityAvg = 1000
			} else {
				stat.ActivityAvg += uint16(delta)
			}
		} else {
			if int(stat.ActivityAvg)+delta < 50 {
				stat.ActivityAvg = 50
			} else {
				stat.ActivityAvg = uint16(int(stat.ActivityAvg) + delta)
			}
		}
	}

	totalActive := uint32(0)
	totalConnections := uint64(0)
	weightSum := uint64(0)
	plasticitySum := float64(0)

	for region := BrainRegion(0); region < BrainRegionMax; region++ {
		stat := &b.Stats.RegionStats[region]
		activeRatio := float64(stat.ActivityAvg) / 1000.0
		activeCells := uint32(float64(stat.CellCount) * activeRatio * 0.1)
		totalActive += activeCells
		totalConnections += uint64(stat.CellCount) * uint64(stat.WeightAvg) / 100
		weightSum += uint64(stat.WeightAvg)
		plasticitySum += float64(stat.ActivityAvg) / 1000.0
	}

	b.Stats.ActiveCells = totalActive
	b.Stats.Connections = totalConnections
	b.Stats.ActivityLevel = float64(totalActive) / float64(b.Stats.TotalCells) * 1000
	b.Stats.Plasticity = plasticitySum / float64(BrainRegionMax)
}

func stageBrainActivity(stage LifeStage) float64 {
	switch stage {
	case LifeStageInfant:
		return 0.3
	case LifeStageChild:
		return 0.7
	case LifeStageAdolescent:
		return 1.0
	case LifeStageYoungAdult:
		return 1.0
	case LifeStageMiddleAged:
		return 0.9
	case LifeStageElderly:
		return 0.7
	case LifeStageCentenarian:
		return 0.5
	default:
		return 0.8
	}
}

func regionCellRatio(region BrainRegion) float64 {
	switch region {
	case BrainRegionFrontal:
		return 0.2
	case BrainRegionParietal:
		return 0.15
	case BrainRegionTemporal:
		return 0.15
	case BrainRegionOccipital:
		return 0.1
	case BrainRegionHippocampus:
		return 0.05
	case BrainRegionAmygdala:
		return 0.02
	case BrainRegionCerebellum:
		return 0.25
	case BrainRegionBrainstem:
		return 0.08
	default:
		return 0.05
	}
}

func (b *Brain) MemorySize() float64 {
	return 256 + float64(BrainRegionMax)*64
}
