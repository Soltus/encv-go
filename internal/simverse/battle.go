package simverse

import (
	"fmt"
	"math/rand"
	"sync"
)

// ============================================================
// 战斗系统（BattleManager）
//
// 设计说明：
//   - 世界规模达千万级 NPC，无法为每场战斗持久化。BattleManager 维护一个
//     固定容量的"近期战斗"环形列表，并在世界 Tick 中以一定概率随机抽取两名
//     缓存 NPC 模拟对战，使战斗成为持续演化的活系统。
//   - 战斗结算基于 NPCV3 的力量/敏捷/等级属性 + RNG，结果稳定可复现。
//   - 每场战斗会写入双方的个人编年史（ChronEventPersonalFight），并累计胜场。
//   - 战斗为"模拟/记录"，不永久改写缓存 NPC 的 HP（避免污染共享状态）。
//   - 复用 interaction.go 的 Skill/Resource 常量与 entity.go 的 NPCV3 类型。
// ============================================================

// BattleRecord 单场战斗记录
type BattleRecord struct {
	ID           uint64 `json:"id"`
	Tick         uint32 `json:"tick"`
	AttackerID   uint64 `json:"attacker_id"`
	AttackerName string `json:"attacker_name"`
	DefenderID   uint64 `json:"defender_id"`
	DefenderName string `json:"defender_name"`
	WinnerID     uint64 `json:"winner_id"`
	LoserID      uint64 `json:"loser_id"`
	Outcome      string `json:"outcome"` // "attacker" | "defender" | "draw"
	Damage       int    `json:"damage"`
	AttackerHP   int    `json:"attacker_hp"`
	DefenderHP   int    `json:"defender_hp"`
	LootGold     uint32 `json:"loot_gold"`
	Log          []string `json:"log"`
}

// BattleManager 维护近期战斗记录与胜场统计
type BattleManager struct {
	mu       sync.Mutex
	chron    *ChronicleManager
	recent   []BattleRecord
	wins     map[uint64]int
	nextID   uint64
	capacity int
}

// NewBattleManager 创建战斗管理器（绑定编年史以便记录战斗事件）
func NewBattleManager(chron *ChronicleManager) *BattleManager {
	return &BattleManager{
		chron:    chron,
		recent:   make([]BattleRecord, 0, 64),
		wins:     make(map[uint64]int),
		capacity: 200,
	}
}

// Simulate 模拟 attacker 与 defender 之间的一场战斗
func (bm *BattleManager) Simulate(attacker, defender *NPCV3, tick uint32, rng *rand.Rand) BattleRecord {
	atkPower := int(attacker.Skills[SkillStrength]) + int(attacker.Skills[SkillAgility])/2 + int(attacker.Level)
	defPower := int(defender.Skills[SkillStrength]) + int(defender.Skills[SkillAgility])/2 + int(defender.Level)

	atkHP := int(attacker.Health)
	defHP := int(defender.Health)
	log := make([]string, 0, 16)

	round := 0
	for atkHP > 0 && defHP > 0 && round < 12 {
		round++
		atkRoll := atkPower + rng.Intn(40) - 20
		defRoll := defPower + rng.Intn(40) - 20

		if atkRoll >= defRoll {
			dmg := 5 + rng.Intn(15)
			if atkRoll > defRoll {
				dmg += (atkRoll - defRoll) / 4
			}
			defHP -= dmg
			if defHP < 0 {
				defHP = 0
			}
			log = append(log, fmt.Sprintf("%s 命中 %s，造成 %d 点伤害", attacker.Name, defender.Name, dmg))
		} else {
			log = append(log, fmt.Sprintf("%s 的攻击被 %s 格挡", attacker.Name, defender.Name))
		}

		if defHP <= 0 {
			break
		}

		if defRoll >= atkRoll {
			dmg := 5 + rng.Intn(15)
			if defRoll > atkRoll {
				dmg += (defRoll - atkRoll) / 4
			}
			atkHP -= dmg
			if atkHP < 0 {
				atkHP = 0
			}
			log = append(log, fmt.Sprintf("%s 反击 %s，造成 %d 点伤害", defender.Name, attacker.Name, dmg))
		}
	}

	rec := BattleRecord{
		ID:           bm.nextID,
		Tick:         tick,
		AttackerID:   attacker.ID,
		AttackerName: attacker.Name,
		DefenderID:   defender.ID,
		DefenderName: defender.Name,
		AttackerHP:   atkHP,
		DefenderHP:   defHP,
	}
	bm.nextID++

	switch {
	case atkHP > defHP:
		rec.WinnerID = attacker.ID
		rec.LoserID = defender.ID
		rec.Outcome = "attacker"
		rec.Damage = int(defender.Health) - defHP
		loot := defender.Bank[ResGold] / 10
		if loot > 0 {
			rec.LootGold = loot
		}
		bm.wins[attacker.ID]++
		log = append(log, fmt.Sprintf("%s 获胜，掠夺 %d 金币", attacker.Name, loot))
	case defHP > atkHP:
		rec.WinnerID = defender.ID
		rec.LoserID = attacker.ID
		rec.Outcome = "defender"
		rec.Damage = int(attacker.Health) - atkHP
		loot := attacker.Bank[ResGold] / 10
		if loot > 0 {
			rec.LootGold = loot
		}
		bm.wins[defender.ID]++
		log = append(log, fmt.Sprintf("%s 获胜，掠夺 %d 金币", defender.Name, loot))
	default:
		rec.Outcome = "draw"
		rec.Damage = 0
		log = append(log, "势均力敌，不分胜负")
	}
	rec.Log = log

	if bm.chron != nil {
		bm.chron.RecordPersonal(tick, attacker.ID, ChronEventPersonalFight, defender.ID)
		bm.chron.RecordPersonal(tick, defender.ID, ChronEventPersonalFight, attacker.ID)
	}

	bm.mu.Lock()
	bm.recent = append(bm.recent, rec)
	if len(bm.recent) > bm.capacity {
		bm.recent = bm.recent[len(bm.recent)-bm.capacity:]
	}
	bm.mu.Unlock()

	return rec
}

// GetRecent 返回近期战斗（按时间倒序，最多 limit 条）
func (bm *BattleManager) GetRecent(limit int) []BattleRecord {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if limit <= 0 || limit > bm.capacity {
		limit = 20
	}
	n := len(bm.recent)
	start := n - limit
	if start < 0 {
		start = 0
	}
	out := make([]BattleRecord, 0, n-start)
	for i := n - 1; i >= start; i-- {
		out = append(out, bm.recent[i])
	}
	return out
}

// BattleRankEntry 胜场榜条目
type BattleRankEntry struct {
	NPCID uint64 `json:"npc_id"`
	Wins  int    `json:"wins"`
}

// GetRank 返回胜场榜（按胜场降序，最多 limit 名）
func (bm *BattleManager) GetRank(limit int) []BattleRankEntry {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	type wp struct {
		id   uint64
		wins int
	}
	arr := make([]wp, 0, len(bm.wins))
	for id, w := range bm.wins {
		arr = append(arr, wp{id, w})
	}
	// 简单选择排序（规模小）
	for i := 0; i < len(arr); i++ {
		for j := i + 1; j < len(arr); j++ {
			if arr[j].wins > arr[i].wins {
				arr[i], arr[j] = arr[j], arr[i]
			}
		}
	}
	out := make([]BattleRankEntry, 0, limit)
	for i := 0; i < len(arr) && i < limit; i++ {
		out = append(out, BattleRankEntry{NPCID: arr[i].id, Wins: arr[i].wins})
	}
	return out
}

// TotalBattles 已记录的战斗总数
func (bm *BattleManager) TotalBattles() int {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return len(bm.recent)
}
