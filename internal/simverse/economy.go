package simverse

import (
	"math"
	"math/rand"
	"sync"
)

type RegionEconomy struct {
	RegionID uint32
	mu       sync.RWMutex

	basePrices   [ResMax]float64
	supply       [ResMax]float64
	demand       [ResMax]float64
	priceHistory [][]float64
	TradeVolume  float64
}

func NewRegionEconomy(regionID uint32) *RegionEconomy {
	e := &RegionEconomy{
		RegionID: regionID,
		basePrices: [ResMax]float64{
			ResFood:        10,
			ResWood:        15,
			ResStone:       20,
			ResIron:        50,
			ResGold:        1,
			ResCloth:       30,
			ResPotion:      40,
			ResManaCrystal: 100,
			ResHerb:        20,
			ResLeather:     25,
		},
		priceHistory: make([][]float64, ResMax),
	}
	for i := range e.supply {
		e.supply[i] = 100
		e.demand[i] = 100
		e.priceHistory[i] = []float64{e.basePrices[i]}
	}
	return e
}

func (e *RegionEconomy) GetCurrentPrice(r ResourceType) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if r >= ResMax {
		return 0
	}

	base := e.basePrices[r]
	supply := e.supply[r]
	demand := e.demand[r]

	if supply <= 0 {
		return base * 3
	}

	ratio := demand / supply
	price := base * math.Pow(ratio, 0.5)

	price = math.Max(base*0.3, math.Min(base*3, price))
	return math.Round(price*100) / 100
}

func (e *RegionEconomy) GetAllPrices() [ResMax]float64 {
	var prices [ResMax]float64
	for i := ResFood; i < ResMax; i++ {
		prices[i] = e.GetCurrentPrice(i)
	}
	return prices
}

func (e *RegionEconomy) RecordSupply(r ResourceType, amount float64) {
	if r >= ResMax {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.supply[r] += amount
	if e.supply[r] < 1 {
		e.supply[r] = 1
	}
}

func (e *RegionEconomy) RecordDemand(r ResourceType, amount float64) {
	if r >= ResMax {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.demand[r] += amount
	if e.demand[r] < 1 {
		e.demand[r] = 1
	}
}

func (e *RegionEconomy) DecaySupplyDemand() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range e.supply {
		e.supply[i] = e.supply[i]*0.95 + 5
		e.demand[i] = e.demand[i]*0.95 + 5
	}
}

func (e *RegionEconomy) RecordPriceHistory() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i := ResFood; i < ResMax; i++ {
		price := e.GetCurrentPrice(i)
		e.priceHistory[i] = append(e.priceHistory[i], price)
		if len(e.priceHistory[i]) > 50 {
			e.priceHistory[i] = e.priceHistory[i][1:]
		}
	}
}

func (e *RegionEconomy) GetPriceHistory(r ResourceType) []float64 {
	if r >= ResMax {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]float64{}, e.priceHistory[r]...)
}

func (e *RegionEconomy) GetSupplyDemand() (supply, demand [ResMax]float64) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.supply, e.demand
}

type EconomyManager struct {
	regions map[uint32]*RegionEconomy
	mu      sync.RWMutex
	rng     *rand.Rand
}

func NewEconomyManager(rng *rand.Rand) *EconomyManager {
	return &EconomyManager{
		regions: make(map[uint32]*RegionEconomy),
		rng:     rng,
	}
}

func (em *EconomyManager) GetRegionEconomy(regionID uint32) *RegionEconomy {
	em.mu.RLock()
	e, ok := em.regions[regionID]
	em.mu.RUnlock()

	if ok {
		return e
	}

	em.mu.Lock()
	defer em.mu.Unlock()

	if e, ok = em.regions[regionID]; ok {
		return e
	}

	e = NewRegionEconomy(regionID)
	em.regions[regionID] = e
	return e
}

func (em *EconomyManager) Tick(world *FractalWorld) {
	em.mu.RLock()
	regions := make([]*RegionEconomy, 0, len(em.regions))
	for _, e := range em.regions {
		regions = append(regions, e)
	}
	em.mu.RUnlock()

	for _, e := range regions {
		e.DecaySupplyDemand()
		e.RecordPriceHistory()
	}

	em.processNPCProduction(world)
	em.processNPCConsumption(world)
}

func (em *EconomyManager) processNPCProduction(world *FractalWorld) {
	npcs := world.GetCachedNPCs()
	for _, npc := range npcs {
		if !npc.IsAlive {
			continue
		}

		regionID := npc.RegionID
		e := em.GetRegionEconomy(regionID)

		prof := npc.Profession
		level := float64(npc.Level)
		switch prof {
		case ProfFarmer:
			e.RecordSupply(ResFood, 2+level*0.5)
		case ProfWoodcutter:
			e.RecordSupply(ResWood, 1.5+level*0.3)
		case ProfMiner:
			e.RecordSupply(ResStone, 1+level*0.3)
			e.RecordSupply(ResIron, 0.5+level*0.2)
		case ProfBlacksmith:
			e.RecordDemand(ResStone, 0.5)
			e.RecordSupply(ResIron, 0.8+level*0.2)
		case ProfTailor:
			e.RecordSupply(ResCloth, 1+level*0.2)
		case ProfMerchant:
			e.TradeVolume += 10 + level*2
		case ProfAlchemist:
			e.RecordDemand(ResHerb, 1)
			e.RecordSupply(ResPotion, 0.5+level*0.2)
		case ProfHealer:
			e.RecordSupply(ResPotion, 0.3+level*0.1)
			e.RecordSupply(ResHerb, 0.5+level*0.2)
		case ProfCook:
			e.RecordDemand(ResFood, 0.5)
			e.RecordSupply(ResFood, 1+level*0.3)
		case ProfScribe:
			e.RecordSupply(ResManaCrystal, 0.2+level*0.1)
		}
	}
}

func (em *EconomyManager) processNPCConsumption(world *FractalWorld) {
	npcs := world.GetCachedNPCs()
	for _, npc := range npcs {
		if !npc.IsAlive {
			continue
		}

		regionID := npc.RegionID
		e := em.GetRegionEconomy(regionID)

		e.RecordDemand(ResFood, 1+float64(npc.Level)*0.1)

		if npc.WealthTier >= 2 {
			e.RecordDemand(ResCloth, 0.2)
		}
		if npc.WealthTier >= 3 {
			e.RecordDemand(ResHerb, 0.1)
			e.RecordDemand(ResPotion, 0.05)
		}
		if npc.WealthTier >= 4 {
			e.RecordDemand(ResManaCrystal, 0.05)
		}
	}
}

func (em *EconomyManager) GetRegionalStats(regionID uint32) map[string]interface{} {
	e := em.GetRegionEconomy(regionID)
	prices := e.GetAllPrices()
	supply, demand := e.GetSupplyDemand()

	priceMap := make(map[string]float64)
	supplyMap := make(map[string]float64)
	demandMap := make(map[string]float64)

	for i := ResFood; i < ResMax; i++ {
		name := i.String()
		priceMap[name] = prices[i]
		supplyMap[name] = math.Round(supply[i]*100) / 100
		demandMap[name] = math.Round(demand[i]*100) / 100
	}

	return map[string]interface{}{
		"region_id":    regionID,
		"prices":       priceMap,
		"supply":       supplyMap,
		"demand":       demandMap,
		"trade_volume": e.TradeVolume,
	}
}

func (em *EconomyManager) GetTopNPCsByWealth(world *FractalWorld, count int) []map[string]interface{} {
	npcs := world.GetCachedNPCs()

	type npcWealth struct {
		id     uint64
		name   string
		level  uint16
		prof   ProfessionType
		wealth int
	}

	wealthList := make([]npcWealth, 0, len(npcs))
	for _, npc := range npcs {
		if !npc.IsAlive {
			continue
		}
		wealth := int(npc.WealthTier)*1000 + int(npc.Level)*50
		wealthList = append(wealthList, npcWealth{
			id:     npc.ID,
			name:   npc.Name,
			level:  npc.Level,
			prof:   npc.Profession,
			wealth: wealth,
		})
	}

	for i := 0; i < len(wealthList)-1; i++ {
		for j := i + 1; j < len(wealthList); j++ {
			if wealthList[j].wealth > wealthList[i].wealth {
				wealthList[i], wealthList[j] = wealthList[j], wealthList[i]
			}
		}
	}

	if count > len(wealthList) {
		count = len(wealthList)
	}

	result := make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		result[i] = map[string]interface{}{
			"id":          wealthList[i].id,
			"name":        wealthList[i].name,
			"level":       wealthList[i].level,
			"profession":  wealthList[i].prof.String(),
			"wealth":      wealthList[i].wealth,
			"rank":        i + 1,
		}
	}

	return result
}

type EconomicEvent struct {
	Type     string
	RegionID uint32
	Resource ResourceType
	Price    float64
	Change   float64
	Message  string
}

func (em *EconomyManager) CheckPriceShocks() []EconomicEvent {
	var events []EconomicEvent

	em.mu.RLock()
	regions := make([]*RegionEconomy, 0, len(em.regions))
	for _, e := range em.regions {
		regions = append(regions, e)
	}
	em.mu.RUnlock()

	for _, e := range regions {
		for r := ResFood; r < ResMax; r++ {
			history := e.GetPriceHistory(r)
			if len(history) < 5 {
				continue
			}

			current := history[len(history)-1]
			previous := history[len(history)-5]
			if previous <= 0 {
				continue
			}

			change := (current - previous) / previous
			if math.Abs(change) > 0.3 {
				direction := "上涨"
				if change < 0 {
					direction = "下跌"
				}
				changePct := int(math.Abs(change) * 100)
				events = append(events, EconomicEvent{
					Type:     "price_shock",
					RegionID: e.RegionID,
					Resource: r,
					Price:    current,
					Change:   change,
					Message:  r.String() + "价格" + direction + "了" + itoa(changePct) + "%",
				})
			}
		}
	}

	return events
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
