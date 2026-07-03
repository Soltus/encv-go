package simverse

import (
	"encoding/binary"
	"math/rand"
)

type NPCV3 struct {
	ID             uint64
	Name           string
	Species        SpeciesType
	Gender         Gender
	GenderIdentity GenderIdentity
	SexualOrient   SexualOrientation
	Profession     ProfessionType
	Level          uint16
	Age            uint16
	Health         uint16
	MaxHealth      uint16
	Energy         uint16
	MaxEnergy      uint16
	Mana           uint16
	MaxMana        uint16
	Mood           int8
	Satisfaction   int8
	Skills         Skills
	Inventory      Resources
	Bank           Resources
	OrgID          uint32
	RegionID       uint32
	HomeRegionID   uint32
	BornAt         int32
	DiedAt         int32
	IsAlive        bool
	LastUpdateTick uint32
	Experience     uint32
	WealthTier     uint8
	SocialTier     uint8
	CareerStage    uint8
	LifeStage      LifeStage
	NumChildren    uint8
	NumMarriages   uint8
	FactionRep     [8]int16
	LifeEvents     uint32
}

const NPCV3BaseSize = 160

func NPCV3Size() float64 {
	return float64(NPCV3BaseSize) + 20
}

type CatchUpResult struct {
	TicksSimulated uint32
	EventsOccurred uint32
	AgeDelta       uint16
	SkillGain      Skills
	WealthDelta    Resources
	ProfessionChanged bool
	RelationshipDelta int32
	LifeEvents     []LifeEventType
	Died           bool
	HadChildren    uint8
}

type LifeEventType uint8

const (
	LifeEventBirth LifeEventType = iota
	LifeEventChildhood
	LifeEventAdolescence
	LifeEventComingOfAge
	LifeEventFirstJob
	LifeEventPromotion
	LifeEventCareerChange
	LifeEventMarriage
	LifeEventDivorce
	LifeEventChildBirth
	LifeEventRetirement
	LifeEventAging
	LifeEventIllness
	LifeEventDeath
	LifeEventMax
)

func (n *NPCV3) CatchUp(currentTick uint32, rng *rand.Rand) CatchUpResult {
	result := CatchUpResult{}

	if !n.IsAlive {
		return result
	}

	ticksPassed := currentTick - n.LastUpdateTick
	if ticksPassed == 0 {
		return result
	}

	result.TicksSimulated = ticksPassed

	stage := GetLifeStage(n.Age, n.Species)
	n.LifeStage = stage
	interval := uint32(StageUpdateInterval(stage))

	updates := ticksPassed / interval
	if updates == 0 {
		updates = 1
	}

	eventsPerUpdate := LifeEventCountPerStage(stage)
	totalEvents := float64(updates) * eventsPerUpdate
	result.EventsOccurred = uint32(totalEvents)

	ageDelta := float64(ticksPassed) / 1000.0
	n.Age += uint16(ageDelta)
	result.AgeDelta = uint16(ageDelta)

	newStage := GetLifeStage(n.Age, n.Species)
	if newStage != stage {
		n.LifeStage = newStage
		result.LifeEvents = append(result.LifeEvents, LifeEventAging)

		switch newStage {
		case LifeStageAdolescent:
			result.LifeEvents = append(result.LifeEvents, LifeEventAdolescence)
		case LifeStageYoungAdult:
			result.LifeEvents = append(result.LifeEvents, LifeEventComingOfAge)
			n.Profession = chooseStartingProfession(n, rng)
			result.ProfessionChanged = true
			result.LifeEvents = append(result.LifeEvents, LifeEventFirstJob)
		case LifeStageMiddleAged:
			if rng.Intn(100) < 30 {
				n.Profession = careerChange(n, rng)
				result.ProfessionChanged = true
				result.LifeEvents = append(result.LifeEvents, LifeEventCareerChange)
			} else {
				n.Level = uint16(min(int(n.Level)+1, 50))
				result.LifeEvents = append(result.LifeEvents, LifeEventPromotion)
			}
		case LifeStageElderly:
			result.LifeEvents = append(result.LifeEvents, LifeEventRetirement)
		}
	}

	skillGainRate := stageSkillRate(stage)
	for s := 0; s < int(SkillMax); s++ {
		gain := uint8(float64(updates) * skillGainRate[s] * float64(n.Skills[s]+1) / 255.0)
		if n.Skills[s]+gain > 255 {
			gain = 255 - n.Skills[s]
		}
		n.Skills[s] += gain
		result.SkillGain[s] = gain
	}

	workRate := stageWorkRate(stage)
	primaryRes := GetProfessionResource(n.Profession)
	income := uint32(float64(updates) * workRate * float64(n.Skills[GetProfessionSkill(n.Profession)]) / 100.0)
	n.Bank[primaryRes] += income
	n.Bank[ResGold] += income / 10
	result.WealthDelta[primaryRes] = income
	result.WealthDelta[ResGold] = income / 10

	expense := uint32(float64(updates) * stageExpenseRate(stage))
	if n.Inventory[ResFood] >= expense/10 {
		n.Inventory[ResFood] -= expense / 10
	} else {
		n.Health = uint16(max(1, int(n.Health)-int(updates)))
	}

	mortalityRisk := stageMortalityRate(stage, n.Species) * float64(updates)
	if rng.Float64() < mortalityRisk {
		n.IsAlive = false
		n.DiedAt = int32(currentTick)
		result.Died = true
		result.LifeEvents = append(result.LifeEvents, LifeEventDeath)
	}

	if stage == LifeStageYoungAdult || stage == LifeStageMiddleAged {
		if rng.Float64() < 0.1*float64(updates)/100.0 {
			result.HadChildren = uint8(rng.Intn(3) + 1)
			n.NumChildren += result.HadChildren
			result.LifeEvents = append(result.LifeEvents, LifeEventChildBirth)
		}
	}

	result.RelationshipDelta = int32(float64(updates) * 0.5)

	n.LastUpdateTick = currentTick
	n.LifeEvents += uint32(len(result.LifeEvents))

	return result
}

func chooseStartingProfession(n *NPCV3, rng *rand.Rand) ProfessionType {
	weights := make([]float64, ProfMax)

	for i := ProfessionType(0); i < ProfMax; i++ {
		weights[i] = 1.0
	}

	weights[ProfFarmer] *= 3.0
	weights[ProfMiner] *= 1.5
	weights[ProfWoodcutter] *= 1.5
	weights[ProfMerchant] *= 1.2
	weights[ProfWarrior] *= 1.0
	weights[ProfMage] *= 0.5
	weights[ProfPriest] *= 0.8
	weights[ProfNoble] *= 0.2 * float64(n.SocialTier+1)
	weights[ProfSlave] *= 0.1

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

func careerChange(n *NPCV3, rng *rand.Rand) ProfessionType {
	if rng.Intn(100) < 70 {
		return n.Profession
	}

	options := []ProfessionType{
		ProfMerchant, ProfWarrior, ProfMage, ProfPriest,
		ProfRogue, ProfRanger, ProfAlchemist, ProfScribe,
	}
	return options[rng.Intn(len(options))]
}

func stageSkillRate(stage LifeStage) [SkillMax]float64 {
	switch stage {
	case LifeStageInfant:
		return [SkillMax]float64{0.1, 0.1, 0.1, 0.05, 0.1, 0.05, 0, 0, 0, 0}
	case LifeStageChild:
		return [SkillMax]float64{0.5, 0.5, 1.0, 0.5, 0.8, 0.3, 0.2, 0.1, 0.2, 0.1}
	case LifeStageAdolescent:
		return [SkillMax]float64{2.0, 2.0, 3.0, 1.5, 2.0, 1.5, 1.0, 0.8, 1.0, 0.5}
	case LifeStageYoungAdult:
		return [SkillMax]float64{3.0, 2.5, 2.0, 1.5, 1.5, 2.0, 1.5, 1.0, 2.0, 1.5}
	case LifeStageMiddleAged:
		return [SkillMax]float64{1.5, 1.0, 1.5, 2.0, 1.0, 1.0, 1.0, 0.5, 2.5, 2.0}
	case LifeStageElderly:
		return [SkillMax]float64{0.3, 0.2, 1.0, 1.5, 0.5, 0.2, 0.5, 0.1, 1.0, 1.5}
	case LifeStageCentenarian:
		return [SkillMax]float64{0.1, 0.1, 0.5, 0.8, 0.3, 0.1, 0.2, 0, 0.3, 0.5}
	default:
		return [SkillMax]float64{}
	}
}

func stageWorkRate(stage LifeStage) float64 {
	switch stage {
	case LifeStageInfant:
		return 0
	case LifeStageChild:
		return 0.1
	case LifeStageAdolescent:
		return 1.0
	case LifeStageYoungAdult:
		return 5.0
	case LifeStageMiddleAged:
		return 6.0
	case LifeStageElderly:
		return 1.0
	case LifeStageCentenarian:
		return 0.2
	default:
		return 0
	}
}

func stageExpenseRate(stage LifeStage) float64 {
	switch stage {
	case LifeStageInfant:
		return 0.5
	case LifeStageChild:
		return 1.0
	case LifeStageAdolescent:
		return 2.0
	case LifeStageYoungAdult:
		return 3.0
	case LifeStageMiddleAged:
		return 4.0
	case LifeStageElderly:
		return 2.0
	case LifeStageCentenarian:
		return 1.0
	default:
		return 1.0
	}
}

func stageMortalityRate(stage LifeStage, species SpeciesType) float64 {
	base := [SpeciesMax][LifeStageMax]float64{
		SpeciesHuman: {
			LifeStageInfant:    0.005,
			LifeStageChild:     0.001,
			LifeStageAdolescent: 0.0005,
			LifeStageYoungAdult: 0.001,
			LifeStageMiddleAged: 0.005,
			LifeStageElderly:   0.05,
			LifeStageCentenarian: 0.2,
		},
		SpeciesElf: {
			LifeStageInfant:    0.001,
			LifeStageChild:     0.0001,
			LifeStageAdolescent: 0.0001,
			LifeStageYoungAdult: 0.0002,
			LifeStageMiddleAged: 0.001,
			LifeStageElderly:   0.01,
			LifeStageCentenarian: 0.05,
		},
		SpeciesDwarf: {
			LifeStageInfant:    0.002,
			LifeStageChild:     0.0005,
			LifeStageAdolescent: 0.0002,
			LifeStageYoungAdult: 0.0005,
			LifeStageMiddleAged: 0.002,
			LifeStageElderly:   0.02,
			LifeStageCentenarian: 0.08,
		},
		SpeciesOrc: {
			LifeStageInfant:    0.01,
			LifeStageChild:     0.005,
			LifeStageAdolescent: 0.003,
			LifeStageYoungAdult: 0.005,
			LifeStageMiddleAged: 0.02,
			LifeStageElderly:   0.1,
			LifeStageCentenarian: 0.3,
		},
		SpeciesBeastman: {
			LifeStageInfant:    0.008,
			LifeStageChild:     0.002,
			LifeStageAdolescent: 0.001,
			LifeStageYoungAdult: 0.002,
			LifeStageMiddleAged: 0.01,
			LifeStageElderly:   0.06,
			LifeStageCentenarian: 0.15,
		},
		SpeciesDragonkin: {
			LifeStageInfant:    0.002,
			LifeStageChild:     0.0005,
			LifeStageAdolescent: 0.0002,
			LifeStageYoungAdult: 0.0005,
			LifeStageMiddleAged: 0.001,
			LifeStageElderly:   0.005,
			LifeStageCentenarian: 0.02,
		},
		SpeciesFey: {
			LifeStageInfant:    0.003,
			LifeStageChild:     0.0005,
			LifeStageAdolescent: 0.0002,
			LifeStageYoungAdult: 0.0003,
			LifeStageMiddleAged: 0.001,
			LifeStageElderly:   0.01,
			LifeStageCentenarian: 0.03,
		},
		SpeciesUndead: {
			LifeStageInfant:    0,
			LifeStageChild:     0,
			LifeStageAdolescent: 0,
			LifeStageYoungAdult: 0,
			LifeStageMiddleAged: 0,
			LifeStageElderly:   0.001,
			LifeStageCentenarian: 0.005,
		},
	}
	if int(stage) >= len(base[species]) {
		return 0.01
	}
	return base[species][stage]
}

func (n *NPCV3) MarshalToV3(buf []byte) int {
	if len(buf) < NPCV3BaseSize {
		return 0
	}

	off := 0
	binary.LittleEndian.PutUint64(buf[off:off+8], n.ID)
	off += 8
	buf[off] = byte(n.Species)
	off++
	buf[off] = byte(n.Gender)
	off++
	buf[off] = byte(n.GenderIdentity)
	off++
	buf[off] = byte(n.SexualOrient)
	off++
	binary.LittleEndian.PutUint16(buf[off:off+2], uint16(n.Profession))
	off += 2
	binary.LittleEndian.PutUint16(buf[off:off+2], n.Level)
	off += 2
	binary.LittleEndian.PutUint16(buf[off:off+2], n.Age)
	off += 2
	binary.LittleEndian.PutUint16(buf[off:off+2], n.Health)
	off += 2
	binary.LittleEndian.PutUint16(buf[off:off+2], n.MaxHealth)
	off += 2
	binary.LittleEndian.PutUint16(buf[off:off+2], n.Energy)
	off += 2
	binary.LittleEndian.PutUint16(buf[off:off+2], n.MaxEnergy)
	off += 2
	binary.LittleEndian.PutUint16(buf[off:off+2], n.Mana)
	off += 2
	binary.LittleEndian.PutUint16(buf[off:off+2], n.MaxMana)
	off += 2
	buf[off] = byte(n.Mood)
	off++
	buf[off] = byte(n.Satisfaction)
	off++

	for i := 0; i < int(SkillMax); i++ {
		buf[off] = n.Skills[i]
		off++
	}

	for i := 0; i < int(ResMax); i++ {
		binary.LittleEndian.PutUint32(buf[off:off+4], n.Inventory[i])
		off += 4
	}

	for i := 0; i < int(ResMax); i++ {
		binary.LittleEndian.PutUint32(buf[off:off+4], n.Bank[i])
		off += 4
	}

	binary.LittleEndian.PutUint32(buf[off:off+4], n.OrgID)
	off += 4
	binary.LittleEndian.PutUint32(buf[off:off+4], n.RegionID)
	off += 4
	binary.LittleEndian.PutUint32(buf[off:off+4], n.HomeRegionID)
	off += 4
	binary.LittleEndian.PutUint32(buf[off:off+4], uint32(n.BornAt))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:off+4], uint32(n.DiedAt))
	off += 4
	if n.IsAlive {
		buf[off] = 1
	} else {
		buf[off] = 0
	}
	off++
	binary.LittleEndian.PutUint32(buf[off:off+4], n.LastUpdateTick)
	off += 4
	binary.LittleEndian.PutUint32(buf[off:off+4], n.Experience)
	off += 4
	buf[off] = n.WealthTier
	off++
	buf[off] = n.SocialTier
	off++
	buf[off] = n.CareerStage
	off++
	buf[off] = byte(n.LifeStage)
	off++
	buf[off] = n.NumChildren
	off++
	buf[off] = n.NumMarriages
	off++

	for i := 0; i < 8; i++ {
		binary.LittleEndian.PutUint16(buf[off:off+2], uint16(n.FactionRep[i]))
		off += 2
	}

	binary.LittleEndian.PutUint32(buf[off:off+4], n.LifeEvents)
	off += 4

	nameLen := len(n.Name)
	if nameLen > 50 {
		nameLen = 50
	}
	buf[off] = byte(nameLen)
	off++
	copy(buf[off:off+nameLen], n.Name[:nameLen])
	off += nameLen

	return off
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
