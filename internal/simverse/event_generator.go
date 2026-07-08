package simverse

import (
	"math/rand"
)

type GeneratedEventType uint8

const (
	GenEventPersonalIllness GeneratedEventType = iota
	GenEventPersonalPromotion
	GenEventPersonalLearnSkill
	GenEventPersonalShopping
	GenEventPersonalAccident
	GenEventInteractionConversation
	GenEventInteractionTrade
	GenEventInteractionConflict
	GenEventInteractionCooperation
	GenEventInteractionFriendship
	GenEventOrgMeeting
	GenEventOrgElection
	GenEventRegionFestival
	GenEventRegionDisaster
	GenEventRegionMarket
	GenEventTypeMax
)

func (e GeneratedEventType) String() string {
	switch e {
	case GenEventPersonalIllness:
		return "personal_illness"
	case GenEventPersonalPromotion:
		return "personal_promotion"
	case GenEventPersonalLearnSkill:
		return "personal_learn_skill"
	case GenEventPersonalShopping:
		return "personal_shopping"
	case GenEventPersonalAccident:
		return "personal_accident"
	case GenEventInteractionConversation:
		return "interaction_conversation"
	case GenEventInteractionTrade:
		return "interaction_trade"
	case GenEventInteractionConflict:
		return "interaction_conflict"
	case GenEventInteractionCooperation:
		return "interaction_cooperation"
	case GenEventInteractionFriendship:
		return "interaction_friendship"
	case GenEventOrgMeeting:
		return "org_meeting"
	case GenEventOrgElection:
		return "org_election"
	case GenEventRegionFestival:
		return "region_festival"
	case GenEventRegionDisaster:
		return "region_disaster"
	case GenEventRegionMarket:
		return "region_market"
	default:
		return "unknown"
	}
}

func (e GeneratedEventType) CN() string {
	switch e {
	case GenEventPersonalIllness:
		return "生病"
	case GenEventPersonalPromotion:
		return "晋升"
	case GenEventPersonalLearnSkill:
		return "学习技能"
	case GenEventPersonalShopping:
		return "购物"
	case GenEventPersonalAccident:
		return "意外事件"
	case GenEventInteractionConversation:
		return "对话交流"
	case GenEventInteractionTrade:
		return "交易往来"
	case GenEventInteractionConflict:
		return "冲突争执"
	case GenEventInteractionCooperation:
		return "合作共事"
	case GenEventInteractionFriendship:
		return "结为好友"
	case GenEventOrgMeeting:
		return "组织会议"
	case GenEventOrgElection:
		return "组织选举"
	case GenEventRegionFestival:
		return "地区节日"
	case GenEventRegionDisaster:
		return "地区灾害"
	case GenEventRegionMarket:
		return "市场波动"
	default:
		return "未知事件"
	}
}

func (e GeneratedEventType) Category() string {
	switch e {
	case GenEventPersonalIllness, GenEventPersonalPromotion,
		GenEventPersonalLearnSkill, GenEventPersonalShopping,
		GenEventPersonalAccident:
		return "personal"
	case GenEventInteractionConversation, GenEventInteractionTrade,
		GenEventInteractionConflict, GenEventInteractionCooperation,
		GenEventInteractionFriendship:
		return "interaction"
	case GenEventOrgMeeting, GenEventOrgElection:
		return "org"
	case GenEventRegionFestival, GenEventRegionDisaster, GenEventRegionMarket:
		return "region"
	default:
		return "unknown"
	}
}

func (e GeneratedEventType) BaseWeight() int {
	switch e {
	case GenEventPersonalShopping:
		return 30
	case GenEventPersonalLearnSkill:
		return 20
	case GenEventPersonalIllness:
		return 5
	case GenEventPersonalPromotion:
		return 3
	case GenEventPersonalAccident:
		return 2
	case GenEventInteractionConversation:
		return 25
	case GenEventInteractionTrade:
		return 15
	case GenEventInteractionCooperation:
		return 8
	case GenEventInteractionConflict:
		return 5
	case GenEventInteractionFriendship:
		return 3
	case GenEventOrgMeeting:
		return 4
	case GenEventOrgElection:
		return 1
	case GenEventRegionMarket:
		return 6
	case GenEventRegionFestival:
		return 2
	case GenEventRegionDisaster:
		return 1
	default:
		return 1
	}
}

type GeneratedEvent struct {
	Type        GeneratedEventType
	EntityID    uint64
	TargetID    uint64
	RegionID    uint32
	OrgID       uint32
	Description string
	Importance  uint8
}

type EventGenerator struct {
	baseRate     float64
	personalMul  float64
	interactMul  float64
	orgMul       float64
	regionMul    float64
}

func NewEventGenerator() *EventGenerator {
	return &EventGenerator{
		baseRate:    0.01,
		personalMul: 1.0,
		interactMul: 0.8,
		orgMul:      0.5,
		regionMul:   0.3,
	}
}

func (eg *EventGenerator) SetRates(baseRate float64, personalMul float64, interactMul float64, orgMul float64, regionMul float64) {
	eg.baseRate = baseRate
	eg.personalMul = personalMul
	eg.interactMul = interactMul
	eg.orgMul = orgMul
	eg.regionMul = regionMul
}

func (eg *EventGenerator) calculateNPCEventProbability(npc *NPCV3, behavior BehaviorType) float64 {
	if !npc.IsAlive {
		return 0
	}

	baseProb := eg.baseRate * eg.personalMul

	if npc.Health < 30 {
		baseProb *= 1.5
	}
	if npc.Energy < 20 {
		baseProb *= 1.2
	}
	if npc.Mood < 30 {
		baseProb *= 1.3
	}

	switch behavior {
	case BehaviorWork:
		baseProb *= 1.2
	case BehaviorSocialize:
		baseProb *= 1.5
	case BehaviorSleep:
		baseProb *= 0.3
	case BehaviorRest:
		baseProb *= 0.8
	}

	switch npc.LifeStage {
	case LifeStageInfant:
		baseProb *= 0.5
	case LifeStageChild:
		baseProb *= 0.7
	case LifeStageAdolescent:
		baseProb *= 1.2
	case LifeStageYoungAdult:
		baseProb *= 1.3
	case LifeStageMiddleAged:
		baseProb *= 1.1
	case LifeStageElderly:
		baseProb *= 0.9
	case LifeStageCentenarian:
		baseProb *= 0.7
	}

	return baseProb
}

func (eg *EventGenerator) selectPersonalEventType(npc *NPCV3, behavior BehaviorType, rng *rand.Rand) GeneratedEventType {
	weights := make([]int, GenEventTypeMax)

	for i := 0; i < int(GenEventTypeMax); i++ {
		et := GeneratedEventType(i)
		if et.Category() == "personal" {
			weights[i] = et.BaseWeight()
		}
	}

	if npc.Health < 30 {
		weights[GenEventPersonalIllness] *= 3
	}
	if npc.Health > 80 {
		weights[GenEventPersonalIllness] /= 2
	}

	switch behavior {
	case BehaviorWork:
		weights[GenEventPersonalPromotion] *= 2
		weights[GenEventPersonalLearnSkill] *= 2
	case BehaviorSocialize:
		weights[GenEventPersonalShopping] *= 2
	case BehaviorExplore:
		weights[GenEventPersonalAccident] *= 3
		weights[GenEventPersonalLearnSkill] *= 2
	case BehaviorRest:
		weights[GenEventPersonalShopping] *= 2
	case BehaviorSleep:
		weights[GenEventPersonalIllness] *= 2
	}

	switch npc.LifeStage {
	case LifeStageAdolescent, LifeStageYoungAdult:
		weights[GenEventPersonalLearnSkill] *= 2
	case LifeStageMiddleAged:
		weights[GenEventPersonalPromotion] *= 2
	case LifeStageElderly, LifeStageCentenarian:
		weights[GenEventPersonalIllness] *= 2
		weights[GenEventPersonalPromotion] /= 3
	}

	if npc.Profession == ProfNone {
		weights[GenEventPersonalPromotion] = 0
	}

	total := 0
	for _, w := range weights {
		total += w
	}
	if total == 0 {
		return GenEventPersonalShopping
	}

	randVal := rng.Intn(total)
	cumulative := 0
	for i, w := range weights {
		cumulative += w
		if randVal < cumulative {
			return GeneratedEventType(i)
		}
	}

	return GenEventPersonalShopping
}

func (eg *EventGenerator) selectInteractionEventType(npc *NPCV3, behavior BehaviorType, rng *rand.Rand) GeneratedEventType {
	weights := make([]int, GenEventTypeMax)

	for i := 0; i < int(GenEventTypeMax); i++ {
		et := GeneratedEventType(i)
		if et.Category() == "interaction" {
			weights[i] = et.BaseWeight()
		}
	}

	switch behavior {
	case BehaviorSocialize:
		weights[GenEventInteractionConversation] *= 3
		weights[GenEventInteractionFriendship] *= 2
	case BehaviorTrade:
		weights[GenEventInteractionTrade] *= 4
	case BehaviorWork:
		weights[GenEventInteractionCooperation] *= 2
		weights[GenEventInteractionConflict] *= 2
	case BehaviorExplore:
		weights[GenEventInteractionTrade] *= 2
		weights[GenEventInteractionConflict] *= 2
	}

	total := 0
	for _, w := range weights {
		total += w
	}
	if total == 0 {
		return GenEventInteractionConversation
	}

	randVal := rng.Intn(total)
	cumulative := 0
	for i, w := range weights {
		cumulative += w
		if randVal < cumulative {
			return GeneratedEventType(i)
		}
	}

	return GenEventInteractionConversation
}

func (eg *EventGenerator) GenerateForNPC(npc *NPCV3, behavior BehaviorState, currentTick uint32, rng *rand.Rand) []GeneratedEvent {
	var events []GeneratedEvent

	prob := eg.calculateNPCEventProbability(npc, behavior.CurrentBehavior)

	if rng.Float64() < prob {
		eventType := eg.selectPersonalEventType(npc, behavior.CurrentBehavior, rng)
		evt := GeneratedEvent{
			Type:       eventType,
			EntityID:   npc.ID,
			RegionID:   npc.RegionID,
			OrgID:      npc.OrgID,
			Importance: 1,
		}
		events = append(events, evt)
	}

	if rng.Float64() < prob*eg.interactMul {
		eventType := eg.selectInteractionEventType(npc, behavior.CurrentBehavior, rng)
		evt := GeneratedEvent{
			Type:       eventType,
			EntityID:   npc.ID,
			TargetID:   0,
			RegionID:   npc.RegionID,
			OrgID:      npc.OrgID,
			Importance: 1,
		}
		events = append(events, evt)
	}

	return events
}

func (eg *EventGenerator) GenerateBatch(npcs []*NPCV3, behaviorStates map[uint64]*BehaviorState, currentTick uint32, rng *rand.Rand) []GeneratedEvent {
	var allEvents []GeneratedEvent

	for _, npc := range npcs {
		if !npc.IsAlive {
			continue
		}

		var bs BehaviorState
		if behaviorStates != nil {
			if s, ok := behaviorStates[npc.ID]; ok {
				bs = *s
			}
		}

		npcEvents := eg.GenerateForNPC(npc, bs, currentTick, rng)
		allEvents = append(allEvents, npcEvents...)
	}

	return allEvents
}

func (eg *EventGenerator) GenerateOrgEvent(orgID uint32, currentTick uint32, rng *rand.Rand, activeMembers int) *GeneratedEvent {
	baseProb := eg.baseRate * eg.orgMul
	if activeMembers > 0 {
		baseProb *= float64(activeMembers) / 50.0
		if baseProb > 0.5 {
			baseProb = 0.5
		}
	}

	if rng.Float64() >= baseProb {
		return nil
	}

	weights := map[GeneratedEventType]int{
		GenEventOrgMeeting:  GenEventOrgMeeting.BaseWeight(),
		GenEventOrgElection: GenEventOrgElection.BaseWeight(),
	}

	total := 0
	for _, w := range weights {
		total += w
	}
	if total == 0 {
		return nil
	}

	randVal := rng.Intn(total)
	cumulative := 0
	var selectedType GeneratedEventType
	for et, w := range weights {
		cumulative += w
		if randVal < cumulative {
			selectedType = et
			break
		}
	}

	return &GeneratedEvent{
		Type:       selectedType,
		OrgID:      orgID,
		Importance: 2,
	}
}

func (eg *EventGenerator) GenerateRegionEvent(regionID uint32, currentTick uint32, rng *rand.Rand, population int) *GeneratedEvent {
	baseProb := eg.baseRate * eg.regionMul
	if population > 0 {
		baseProb *= float64(population) / 1000.0
		if baseProb > 0.3 {
			baseProb = 0.3
		}
	}

	if rng.Float64() >= baseProb {
		return nil
	}

	weights := map[GeneratedEventType]int{
		GenEventRegionFestival: GenEventRegionFestival.BaseWeight(),
		GenEventRegionDisaster: GenEventRegionDisaster.BaseWeight(),
		GenEventRegionMarket:   GenEventRegionMarket.BaseWeight(),
	}

	total := 0
	for _, w := range weights {
		total += w
	}
	if total == 0 {
		return nil
	}

	randVal := rng.Intn(total)
	cumulative := 0
	var selectedType GeneratedEventType
	for et, w := range weights {
		cumulative += w
		if randVal < cumulative {
			selectedType = et
			break
		}
	}

	return &GeneratedEvent{
		Type:       selectedType,
		RegionID:   regionID,
		Importance: 3,
	}
}

func countEventsByCategory(events []GeneratedEvent) map[string]int {
	result := map[string]int{
		"personal":    0,
		"interaction": 0,
		"org":         0,
		"region":      0,
	}
	for _, e := range events {
		cat := e.Type.Category()
		result[cat]++
	}
	return result
}
