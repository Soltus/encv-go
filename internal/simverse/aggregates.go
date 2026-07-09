package simverse

import (
	"sort"
)

// ============================================================
// 聚合视图 — 从缓存 NPC 派生区域 / 组织级别的统计
//
// 说明：当前引擎并未独立存储 Organization / Region 实体，
// 区域与组织数据均从 NPC 的 RegionID / OrgID 字段聚合得到。
// 这样既能复用已有的经济系统（EconomyManager 按 regionID 索引），
// 又能让前端在没有独立实体存储的情况下拿到有意义的聚合结果。
// ============================================================

// RegionAggregate 区域级聚合（派生自缓存 NPC + 区域经济）
type RegionAggregate struct {
	RegionID      uint32             `json:"region_id"`
	NPCCount      int                `json:"npc_count"`
	AliveCount    int                `json:"alive_count"`
	Population    int                `json:"population"`
	AvgLevel      float64            `json:"avg_level"`
	AvgWealthTier float64            `json:"avg_wealth_tier"`
	Economy       map[string]interface{} `json:"economy,omitempty"`
}

type regionAcc struct {
	regionID   uint32
	npcCount   int
	aliveCount int
	levelSum   float64
	wealthSum  float64
}

// GetRegionAggregates 返回所有出现过的区域聚合
func (fw *FractalWorld) GetRegionAggregates() []RegionAggregate {
	npcs := fw.GetCachedNPCs()
	accs := make(map[uint32]*regionAcc)
	for _, npc := range npcs {
		acc := accs[npc.RegionID]
		if acc == nil {
			acc = &regionAcc{regionID: npc.RegionID}
			accs[npc.RegionID] = acc
		}
		acc.npcCount++
		if npc.IsAlive {
			acc.aliveCount++
		}
		acc.levelSum += float64(npc.Level)
		acc.wealthSum += float64(npc.WealthTier)
	}

	em := fw.EconomyManager()
	result := make([]RegionAggregate, 0, len(accs))
	for _, acc := range accs {
		agg := RegionAggregate{
			RegionID:   acc.regionID,
			NPCCount:   acc.npcCount,
			AliveCount: acc.aliveCount,
			Population: acc.aliveCount,
		}
		if acc.npcCount > 0 {
			agg.AvgLevel = round2(acc.levelSum / float64(acc.npcCount))
			agg.AvgWealthTier = round2(acc.wealthSum / float64(acc.npcCount))
		}
		if em != nil {
			agg.Economy = em.GetRegionalStats(acc.regionID)
		}
		result = append(result, agg)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].RegionID < result[j].RegionID
	})
	return result
}

// GetRegionAggregate 返回指定区域的聚合
func (fw *FractalWorld) GetRegionAggregate(id uint32) (RegionAggregate, bool) {
	aggs := fw.GetRegionAggregates()
	for _, a := range aggs {
		if a.RegionID == id {
			return a, true
		}
	}
	return RegionAggregate{}, false
}

// OrgAggregate 组织级聚合（派生自缓存 NPC 的 OrgID）
type OrgAggregate struct {
	OrgID          uint32         `json:"org_id"`
	Name           string         `json:"name"`
	OrgType        string         `json:"org_type"`
	MemberCount    int            `json:"member_count"`
	AliveCount     int            `json:"alive_count"`
	RegionDist     map[uint32]int `json:"region_distribution"`
	AvgLevel       float64        `json:"avg_level"`
	AvgWealthTier  float64        `json:"avg_wealth_tier"`
	AvgCareerStage float64        `json:"avg_career_stage"`
}

type orgAcc struct {
	orgID       uint32
	memberCount int
	aliveCount  int
	levelSum    float64
	wealthSum   float64
	careerSum   float64
	regionDist  map[uint32]int
}

func orgName(orgID uint32) string {
	if orgID == 0 {
		return "无阵营"
	}
	return OrgIDToType(orgID).String() + "#" + itoaInt(int(orgID))
}

func itoaInt(v int) string {
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

// GetOrgAggregates 返回所有出现过的组织聚合
func (fw *FractalWorld) GetOrgAggregates() []OrgAggregate {
	npcs := fw.GetCachedNPCs()
	accs := make(map[uint32]*orgAcc)
	for _, npc := range npcs {
		acc := accs[npc.OrgID]
		if acc == nil {
			acc = &orgAcc{orgID: npc.OrgID, regionDist: make(map[uint32]int)}
			accs[npc.OrgID] = acc
		}
		acc.memberCount++
		if npc.IsAlive {
			acc.aliveCount++
		}
		acc.levelSum += float64(npc.Level)
		acc.wealthSum += float64(npc.WealthTier)
		acc.careerSum += float64(npc.CareerStage)
		acc.regionDist[npc.RegionID]++
	}

	result := make([]OrgAggregate, 0, len(accs))
	for _, acc := range accs {
		agg := OrgAggregate{
			OrgID:       acc.orgID,
			Name:        orgName(acc.orgID),
			OrgType:     OrgIDToType(acc.orgID).String(),
			MemberCount: acc.memberCount,
			AliveCount:  acc.aliveCount,
			RegionDist:  acc.regionDist,
		}
		if acc.memberCount > 0 {
			agg.AvgLevel = round2(acc.levelSum / float64(acc.memberCount))
			agg.AvgWealthTier = round2(acc.wealthSum / float64(acc.memberCount))
			agg.AvgCareerStage = round2(acc.careerSum / float64(acc.memberCount))
		}
		result = append(result, agg)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].OrgID < result[j].OrgID
	})
	return result
}

// GetOrgAggregate 返回指定组织的聚合
func (fw *FractalWorld) GetOrgAggregate(id uint32) (OrgAggregate, bool) {
	aggs := fw.GetOrgAggregates()
	for _, a := range aggs {
		if a.OrgID == id {
			return a, true
		}
	}
	return OrgAggregate{}, false
}

// GetOrgMembers 返回指定组织的成员（分页），按 ID 升序
func (fw *FractalWorld) GetOrgMembers(orgID uint32, page, pageSize int) ([]*NPCV3, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	npcs := fw.GetCachedNPCs()
	members := make([]*NPCV3, 0, len(npcs))
	for _, npc := range npcs {
		if npc.OrgID == orgID {
			members = append(members, npc)
		}
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].ID < members[j].ID
	})

	total := len(members)
	start := (page - 1) * pageSize
	if start >= total {
		return []*NPCV3{}, total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return members[start:end], total
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
