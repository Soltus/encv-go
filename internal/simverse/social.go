package simverse

import (
	"math/rand"
)

// ============================================================
// 社交关系系统（SocialGraph）
//
// 设计说明：
//   - 世界规模达千万级 NPC，不可能存储全量关系图。
//   - 关系由 NPC 的确定性属性（ID / 婚姻数 / 子女数 / 出生时刻）
//     派生，使用以 NPC.ID 为种子的本地 RNG 生成，结果稳定可复现。
//   - 关系图按 NPC.ID 缓存，避免重复计算；目标 NPC 通过确定性哈希
//     派生，调用方可用 FractalWorld.GetNPC 反查其档案。
//   - 该子系统与 interaction.go 中的 RelationshipGraph / SimulateInteraction
//     共享 entity.go 的 Relationship / RelationshipType 类型。
// ============================================================

// String 返回关系类型的稳定英文 key（前端 i18n 负责本地化）
func (r RelationshipType) String() string {
	switch r {
	case RelStranger:
		return "stranger"
	case RelAcquaint:
		return "acquaintance"
	case RelFriend:
		return "friend"
	case RelLover:
		return "lover"
	case RelSpouse:
		return "spouse"
	case RelParent:
		return "parent"
	case RelChild:
		return "child"
	case RelSibling:
		return "sibling"
	case RelMaster:
		return "master"
	case RelApprentice:
		return "apprentice"
	case RelEnemy:
		return "enemy"
	case RelRival:
		return "rival"
	default:
		return "unknown"
	}
}

// SocialGraph 维护 NPC 关系缓存（确定性派生）
type SocialGraph struct {
	cache map[uint64][]Relationship
}

// NewSocialGraph 创建空社交图
func NewSocialGraph() *SocialGraph {
	return &SocialGraph{cache: make(map[uint64][]Relationship)}
}

// Get 返回（并在首次访问时生成并缓存）指定 NPC 的关系列表
func (g *SocialGraph) Get(npcID uint64, gen func() []Relationship) []Relationship {
	if rels, ok := g.cache[npcID]; ok {
		return rels
	}
	rels := gen()
	g.cache[npcID] = rels
	return rels
}

// NodeCount 已缓存关系节点的数量
func (g *SocialGraph) NodeCount() int {
	return len(g.cache)
}

// socialTargetSpace 派生目标 NPC.ID 的取值范围（1..socialTargetSpace）
const socialTargetSpace = uint64(1_000_000)

// deriveTarget 由源 NPC.ID 与盐值确定性派生一个目标 NPC.ID
func deriveTarget(npcID uint64, salt uint32) uint64 {
	h := npcID*2654435761 + uint64(salt)*40503
	h ^= h >> 17
	h *= 0xed5ad1b1
	h ^= h >> 11
	t := (h % socialTargetSpace) + 1
	if t == npcID {
		t = (t % socialTargetSpace) + 1
	}
	return t
}

// generateRelationships 根据 NPC 的确定性属性派生其社会关系
func generateRelationships(npc *NPCV3) []Relationship {
	rng := rand.New(rand.NewSource(int64(npc.ID)*7919 + 1))
	rels := make([]Relationship, 0, 24)
	salt := uint32(0)

	add := func(relType RelationshipType, affinity int16) {
		tid := deriveTarget(npc.ID, salt)
		salt++
		var lastMeet int32
		if npc.BornAt > 0 {
			lastMeet = int32(rng.Intn(int(npc.BornAt) + 1))
		}
		rels = append(rels, Relationship{
			TargetID: tid,
			RelType:  relType,
			Affinity: affinity,
			LastMeet: lastMeet,
		})
	}

	// 家庭关系
	if npc.NumMarriages > 0 {
		add(RelSpouse, int16(40+rng.Intn(40)))
		add(RelLover, int16(30+rng.Intn(40)))
	}
	children := int(npc.NumChildren)
	if children > 8 {
		children = 8
	}
	for i := 0; i < children; i++ {
		add(RelChild, int16(20+rng.Intn(50)))
	}
	for i := 0; i < 2; i++ { // 父母
		add(RelParent, int16(20+rng.Intn(40)))
	}
	for i := 0; i < rng.Intn(4); i++ { // 兄弟姐妹
		add(RelSibling, int16(10+rng.Intn(50)))
	}

	// 社交关系
	for i := 0; i < 2+rng.Intn(5); i++ { // 朋友 2-6
		add(RelFriend, int16(rng.Intn(60)-10))
	}
	for i := 0; i < 1+rng.Intn(4); i++ { // 熟人 1-4
		add(RelAcquaint, int16(rng.Intn(30)-10))
	}
	if rng.Float64() < 0.3 { // 师徒（单向）
		add(RelMaster, int16(rng.Intn(40)))
	} else if rng.Float64() < 0.3 {
		add(RelApprentice, int16(rng.Intn(40)))
	}

	// 对抗关系
	for i := 0; i < rng.Intn(4); i++ { // 对手 0-3
		add(RelRival, int16(-rng.Intn(40)-5))
	}
	for i := 0; i < rng.Intn(3); i++ { // 敌人 0-2
		add(RelEnemy, int16(-rng.Intn(60)-20))
	}

	return rels
}

// SocialStats 社交关系聚合统计
type SocialStats struct {
	SampledNPCs   int            `json:"sampled_npcs"`
	TotalRelations int           `json:"total_relations"`
	ByType        map[string]int `json:"by_type"`
	ByRegion      map[uint32]int `json:"by_region,omitempty"`
	ByOrg         map[uint32]int `json:"by_org,omitempty"`
}

// Stats 在给定（可选）区域 / 组织过滤下，聚合缓存 NPC 的关系统计
func (g *SocialGraph) Stats(npcs []*NPCV3, regionFilter, orgFilter uint32) SocialStats {
	stats := SocialStats{
		ByType:   make(map[string]int),
		ByRegion: make(map[uint32]int),
		ByOrg:    make(map[uint32]int),
	}
	count := 0
	for _, npc := range npcs {
		if regionFilter != 0 && npc.RegionID != regionFilter {
			continue
		}
		if orgFilter != 0 && npc.OrgID != orgFilter {
			continue
		}
		rels := g.Get(npc.ID, func() []Relationship { return generateRelationships(npc) })
		stats.TotalRelations += len(rels)
		for _, r := range rels {
			stats.ByType[r.RelType.String()]++
		}
		if regionFilter == 0 {
			stats.ByRegion[npc.RegionID] += len(rels)
		}
		if orgFilter == 0 {
			stats.ByOrg[npc.OrgID] += len(rels)
		}
		count++
	}
	stats.SampledNPCs = count
	return stats
}
