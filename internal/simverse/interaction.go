package simverse

import (
	"math/rand"
)

type RelationshipGraph struct {
	nodes map[uint64]map[uint64]Relationship
	count int
}

func NewRelationshipGraph() *RelationshipGraph {
	return &RelationshipGraph{
		nodes: make(map[uint64]map[uint64]Relationship),
	}
}

func (g *RelationshipGraph) Add(a, b uint64, rel Relationship) {
	if g.nodes[a] == nil {
		g.nodes[a] = make(map[uint64]Relationship)
	}
	if g.nodes[b] == nil {
		g.nodes[b] = make(map[uint64]Relationship)
	}
	rel.TargetID = b
	g.nodes[a][b] = rel
	rel2 := rel
	rel2.TargetID = a
	g.nodes[b][a] = rel2
	g.count++
}

func (g *RelationshipGraph) Get(a, b uint64) (Relationship, bool) {
	neighbors, ok := g.nodes[a]
	if !ok {
		return Relationship{}, false
	}
	r, ok := neighbors[b]
	return r, ok
}

func (g *RelationshipGraph) Neighbors(a uint64) []uint64 {
	neighbors, ok := g.nodes[a]
	if !ok {
		return nil
	}
	result := make([]uint64, 0, len(neighbors))
	for id := range neighbors {
		result = append(result, id)
	}
	return result
}

func (g *RelationshipGraph) NodeCount() int {
	return len(g.nodes)
}

func (g *RelationshipGraph) EdgeCount() int {
	return g.count
}

type InteractionResult struct {
	ActorDeltaMood      int8
	TargetDeltaMood     int8
	ActorDeltaAffinity int16
	TargetDeltaAffinity int16
	ResourceDelta       Resources
	EventCount           int
}

func SimulateInteraction(actor, target *NPCV2, eventType EventType, rng *rand.Rand) InteractionResult {
	result := InteractionResult{}

	switch eventType {
	case EventSocialize:
		baseAffinity := int16(rng.Intn(20)) - 8
		charismaDiff := int(actor.Skills[SkillCharisma]) - int(target.Skills[SkillCharisma])
		adjustment := int16(charismaDiff / 10)
		result.ActorDeltaAffinity = baseAffinity + adjustment
		result.TargetDeltaAffinity = baseAffinity - adjustment
		result.ActorDeltaMood = int8(rng.Intn(6) - 2)
		result.TargetDeltaMood = int8(rng.Intn(6) - 2)
		result.EventCount = 1

	case EventTrade:
		tradeSkill := int(actor.Skills[SkillTrading])
		targetTradeSkill := int(target.Skills[SkillTrading])
		value := 10 + rng.Intn(20)
		actorAdvantage := tradeSkill - targetTradeSkill
		goldDelta := value + actorAdvantage/5
		if goldDelta < 1 {
			goldDelta = 1
		}
		result.ResourceDelta[ResGold] = uint32(goldDelta)
		result.ActorDeltaMood = 2
		result.TargetDeltaMood = 1
		result.ActorDeltaAffinity = 3
		result.TargetDeltaAffinity = 2
		result.EventCount = 1

	case EventFight:
		strA := int(actor.Skills[SkillStrength]) + int(actor.Skills[SkillAgility])
		strT := int(target.Skills[SkillStrength]) + int(target.Skills[SkillAgility])
		diff := strA - strT
		damage := 10 + rng.Intn(20) + diff/2
		if damage < 0 {
			damage = 0
		}
		actorDamage := damage / 3
		targetDamage := damage
		if diff > 0 {
			actorDamage = damage / 5
			targetDamage = damage
		} else {
			actorDamage = damage
			targetDamage = damage / 5
		}
		result.ActorDeltaMood = -5
		result.TargetDeltaMood = -8
		result.ActorDeltaAffinity = -20
		result.TargetDeltaAffinity = -25
		result.EventCount = 1
		_ = actorDamage
		_ = targetDamage

	case EventWork:
		profSkill := GetProfessionSkill(actor.Profession)
		output := 5 + int(actor.Skills[profSkill])/5 + rng.Intn(10)
		primaryRes := GetProfessionResource(actor.Profession)
		result.ResourceDelta[primaryRes] = uint32(output)
		actor.Energy = uint16(int(actor.Energy) - 10 - rng.Intn(10))
		if actor.Energy < 0 {
			actor.Energy = 0
		}
		result.ActorDeltaMood = -1
		result.EventCount = 1

	case EventRest:
		actor.Energy = uint16(min(int(actor.Energy)+20+rng.Intn(15), int(actor.MaxEnergy)))
		result.ActorDeltaMood = 3
		result.EventCount = 1

	case EventEat:
		if actor.Inventory[ResFood] > 0 {
			actor.Inventory[ResFood]--
			actor.Health = uint16(min(int(actor.Health)+10, int(actor.MaxHealth)))
			result.ActorDeltaMood = 2
		} else {
			actor.Health = uint16(int(actor.Health) - 5)
			result.ActorDeltaMood = -3
		}
		result.EventCount = 1

	default:
		result.EventCount = 1
	}

	return result
}

func GetProfessionSkill(prof ProfessionType) SkillType {
	switch prof {
	case ProfFarmer, ProfWoodcutter, ProfMiner:
		return SkillStrength
	case ProfBlacksmith, ProfTailor, ProfAlchemist:
		return SkillCrafting
	case ProfMerchant:
		return SkillTrading
	case ProfWarrior, ProfRanger:
		return SkillStrength
	case ProfMage:
		return SkillMagic
	case ProfPriest, ProfHealer:
		return SkillIntelligence
	case ProfRogue:
		return SkillStealth
	default:
		return SkillStrength
	}
}

func GetProfessionResource(prof ProfessionType) ResourceType {
	switch prof {
	case ProfFarmer:
		return ResFood
	case ProfWoodcutter:
		return ResWood
	case ProfMiner:
		return ResStone
	case ProfBlacksmith:
		return ResIron
	case ProfTailor:
		return ResCloth
	case ProfMerchant:
		return ResGold
	case ProfAlchemist:
		return ResPotion
	case ProfCook:
		return ResFood
	default:
		return ResGold
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
