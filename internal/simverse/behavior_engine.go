package simverse

import (
	"math/rand"
)

type BehaviorType uint8

const (
	BehaviorIdle BehaviorType = iota
	BehaviorWork
	BehaviorRest
	BehaviorEat
	BehaviorSleep
	BehaviorSocialize
	BehaviorExplore
	BehaviorTrade
	BehaviorMax
)

func (b BehaviorType) String() string {
	switch b {
	case BehaviorIdle:
		return "idle"
	case BehaviorWork:
		return "work"
	case BehaviorRest:
		return "rest"
	case BehaviorEat:
		return "eat"
	case BehaviorSleep:
		return "sleep"
	case BehaviorSocialize:
		return "socialize"
	case BehaviorExplore:
		return "explore"
	case BehaviorTrade:
		return "trade"
	default:
		return "unknown"
	}
}

func (b BehaviorType) CN() string {
	switch b {
	case BehaviorIdle:
		return "空闲"
	case BehaviorWork:
		return "工作"
	case BehaviorRest:
		return "休息"
	case BehaviorEat:
		return "进食"
	case BehaviorSleep:
		return "睡眠"
	case BehaviorSocialize:
		return "社交"
	case BehaviorExplore:
		return "探索"
	case BehaviorTrade:
		return "交易"
	default:
		return "未知"
	}
}

type NeedSystem struct {
	Hunger      uint8
	Energy      uint8
	Social      uint8
	Achievement uint8
}

func (n *NeedSystem) Clone() NeedSystem {
	return *n
}

type BehaviorState struct {
	CurrentBehavior  BehaviorType
	BehaviorStartTick uint32
	BehaviorDuration uint32
	TargetID         uint64
	TargetPosX       int32
	TargetPosY       int32
	Needs            NeedSystem
}

type BehaviorEngine struct {
	tickRate uint32
}

func NewBehaviorEngine() *BehaviorEngine {
	return &BehaviorEngine{
		tickRate: 1,
	}
}

func (be *BehaviorEngine) InitNPC(npc *NPCV3, rng *rand.Rand) BehaviorState {
	state := BehaviorState{
		CurrentBehavior:   BehaviorIdle,
		BehaviorStartTick: 0,
		BehaviorDuration:  0,
		Needs: NeedSystem{
			Hunger:      uint8(rng.Intn(30) + 10),
			Energy:      uint8(rng.Intn(20) + 70),
			Social:      uint8(rng.Intn(40) + 20),
			Achievement: uint8(rng.Intn(30) + 30),
		},
	}
	return state
}

func (be *BehaviorEngine) TickNeeds(state *BehaviorState, deltaTick uint32) {
	if deltaTick == 0 {
		return
	}

	hungerRate := uint8(1)
	energyRate := uint8(1)
	socialRate := uint8(1)
	achievementRate := uint8(1)

	switch state.CurrentBehavior {
	case BehaviorSleep:
		hungerRate = 1
		energyRate = 0
		socialRate = 1
		achievementRate = 1
	case BehaviorWork:
		hungerRate = 2
		energyRate = 2
		socialRate = 2
		achievementRate = 0
	case BehaviorEat:
		hungerRate = 0
		energyRate = 1
		socialRate = 1
		achievementRate = 1
	case BehaviorRest:
		hungerRate = 1
		energyRate = 0
		socialRate = 2
		achievementRate = 2
	case BehaviorSocialize:
		hungerRate = 1
		energyRate = 2
		socialRate = 0
		achievementRate = 1
	case BehaviorExplore:
		hungerRate = 2
		energyRate = 2
		socialRate = 2
		achievementRate = 2
	case BehaviorTrade:
		hungerRate = 1
		energyRate = 1
		socialRate = 2
		achievementRate = 2
	}

	for i := uint32(0); i < deltaTick; i++ {
		if state.CurrentBehavior == BehaviorSleep {
			if state.Needs.Energy < 100 {
				state.Needs.Energy += 3
				if state.Needs.Energy > 100 {
					state.Needs.Energy = 100
				}
			}
		} else {
			if state.Needs.Energy > energyRate {
				state.Needs.Energy -= energyRate
			} else {
				state.Needs.Energy = 0
			}
		}

		if state.CurrentBehavior == BehaviorEat {
			if state.Needs.Hunger > 10 {
				state.Needs.Hunger -= 15
			} else {
				state.Needs.Hunger = 0
			}
		} else {
			if state.Needs.Hunger < 255-hungerRate {
				state.Needs.Hunger += hungerRate
			} else {
				state.Needs.Hunger = 255
			}
		}

		if state.CurrentBehavior == BehaviorSocialize {
			if state.Needs.Social > 10 {
				state.Needs.Social -= 10
			} else {
				state.Needs.Social = 0
			}
		} else {
			if state.Needs.Social < 255-socialRate {
				state.Needs.Social += socialRate
			} else {
				state.Needs.Social = 255
			}
		}

		if state.CurrentBehavior == BehaviorWork {
			if state.Needs.Achievement > 5 {
				state.Needs.Achievement -= 5
			} else {
				state.Needs.Achievement = 0
			}
		} else {
			if state.Needs.Achievement < 255-achievementRate {
				state.Needs.Achievement += achievementRate
			} else {
				state.Needs.Achievement = 255
			}
		}
	}
}

func (be *BehaviorEngine) DecideNextAction(npc *NPCV3, state *BehaviorState, rng *rand.Rand) BehaviorType {
	if !npc.IsAlive {
		return BehaviorIdle
	}

	hungerScore := int(state.Needs.Hunger) * 3
	energyScore := int(100-state.Needs.Energy) * 4
	socialScore := int(state.Needs.Social) * 2
	achievementScore := int(state.Needs.Achievement) * 2

	stage := npc.LifeStage
	switch stage {
	case LifeStageInfant, LifeStageChild:
		energyScore *= 2
		hungerScore *= 2
	case LifeStageAdolescent, LifeStageYoungAdult:
		socialScore *= 2
		achievementScore *= 2
	case LifeStageMiddleAged:
		achievementScore *= 2
		hungerScore *= 1
	case LifeStageElderly, LifeStageCentenarian:
		energyScore *= 2
		socialScore *= 1
	}

	if npc.Profession == ProfNone {
		achievementScore /= 2
	}

	scores := make([]int, BehaviorMax)
	scores[BehaviorIdle] = 10 + rng.Intn(20)
	scores[BehaviorWork] = achievementScore + rng.Intn(20)
	scores[BehaviorRest] = energyScore/2 + rng.Intn(15)
	scores[BehaviorEat] = hungerScore + rng.Intn(15)
	scores[BehaviorSleep] = energyScore + rng.Intn(10)
	scores[BehaviorSocialize] = socialScore + rng.Intn(20)
	scores[BehaviorExplore] = 5 + rng.Intn(25)
	scores[BehaviorTrade] = 5 + rng.Intn(15)

	if state.Needs.Hunger > 200 {
		scores[BehaviorEat] += 200
	}
	if state.Needs.Energy < 20 {
		scores[BehaviorSleep] += 200
	}
	if state.Needs.Energy < 50 {
		scores[BehaviorRest] += 50
	}

	bestBehavior := BehaviorIdle
	bestScore := -1
	for i := 0; i < int(BehaviorMax); i++ {
		if scores[i] > bestScore {
			bestScore = scores[i]
			bestBehavior = BehaviorType(i)
		}
	}

	return bestBehavior
}

func (be *BehaviorEngine) GetBehaviorDuration(behavior BehaviorType, npc *NPCV3, rng *rand.Rand) uint32 {
	switch behavior {
	case BehaviorSleep:
		return uint32(rng.Intn(200) + 200)
	case BehaviorWork:
		return uint32(rng.Intn(300) + 200)
	case BehaviorEat:
		return uint32(rng.Intn(30) + 20)
	case BehaviorRest:
		return uint32(rng.Intn(100) + 50)
	case BehaviorSocialize:
		return uint32(rng.Intn(100) + 50)
	case BehaviorExplore:
		return uint32(rng.Intn(150) + 50)
	case BehaviorTrade:
		return uint32(rng.Intn(60) + 20)
	case BehaviorIdle:
		return uint32(rng.Intn(50) + 10)
	default:
		return 50
	}
}

func (be *BehaviorEngine) ExecuteBehavior(npc *NPCV3, state *BehaviorState, deltaTick uint32, rng *rand.Rand) {
	if !npc.IsAlive {
		return
	}

	be.TickNeeds(state, deltaTick)

	switch state.CurrentBehavior {
	case BehaviorWork:
		if rng.Float64() < 0.01*float64(deltaTick) {
			skillIdx := rng.Intn(int(SkillMax))
			if npc.Skills[skillIdx] < 255 {
				npc.Skills[skillIdx]++
			}
		}
		if rng.Float64() < 0.005*float64(deltaTick) {
			npc.Experience++
		}
	case BehaviorSleep:
		if state.Needs.Energy >= 95 {
			state.BehaviorDuration = 0
		}
	case BehaviorEat:
		if state.Needs.Hunger <= 10 {
			state.BehaviorDuration = 0
		}
	case BehaviorSocialize:
		if state.Needs.Social <= 20 {
			state.BehaviorDuration = 0
		}
	}
}

func (be *BehaviorEngine) Tick(npc *NPCV3, state *BehaviorState, currentTick uint32, rng *rand.Rand) bool {
	if !npc.IsAlive {
		return false
	}

	elapsed := currentTick - state.BehaviorStartTick
	if elapsed >= state.BehaviorDuration {
		newBehavior := be.DecideNextAction(npc, state, rng)
		state.CurrentBehavior = newBehavior
		state.BehaviorStartTick = currentTick
		state.BehaviorDuration = be.GetBehaviorDuration(newBehavior, npc, rng)
		be.ExecuteBehavior(npc, state, 1, rng)
		return true
	}

	be.ExecuteBehavior(npc, state, 1, rng)
	return false
}
