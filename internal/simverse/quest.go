package simverse

import (
	"math/rand"
	"sync"
)

type QuestType uint8

const (
	QuestTypeDaily     QuestType = 0
	QuestTypeAchieve   QuestType = 1
	QuestTypeStory     QuestType = 2
	QuestTypeEconomy   QuestType = 3
)

type QuestStatus uint8

const (
	QuestStatusLocked   QuestStatus = 0
	QuestStatusActive   QuestStatus = 1
	QuestStatusClaimed  QuestStatus = 2
	QuestStatusExpired  QuestStatus = 3
)

type Quest struct {
	ID          string      `json:"id"`
	Type        QuestType   `json:"type"`
	Title       string      `json:"title"`
	Desc        string      `json:"desc"`
	Icon        string      `json:"icon"`
	Goal        int         `json:"goal"`
	Progress    int         `json:"progress"`
	Reward      QuestReward `json:"reward"`
	Status      QuestStatus `json:"status"`
	SortOrder   int         `json:"sort_order"`
}

type QuestReward struct {
	Diamond int    `json:"diamond"`
	Gold    int    `json:"gold"`
	Exp     int    `json:"exp"`
	Icon    string `json:"icon"`
}

type QuestManager struct {
	mu          sync.RWMutex
	quests      map[string]*Quest
	playerStats PlayerStats
	rng         *rand.Rand
}

type PlayerStats struct {
	TotalTicksObserved int
	TotalNPCsChecked   int
	TotalEconomyChecks int
	TotalGachaPulls    int
	TotalBattles       int
	WorldTicksSeen     int
}

func NewQuestManager(rng *rand.Rand) *QuestManager {
	qm := &QuestManager{
		quests: make(map[string]*Quest),
		rng:    rng,
	}
	qm.initQuests()
	return qm
}

func (qm *QuestManager) initQuests() {
	quests := []*Quest{
		// 日常任务
		{
			ID:        "daily_observe_10",
			Type:      QuestTypeDaily,
			Title:     "观察者",
			Desc:      "查看 10 个 NPC 的详细信息",
			Icon:      "👁️",
			Goal:      10,
			Reward:    QuestReward{Diamond: 50, Gold: 1000, Exp: 20, Icon: "💎"},
			Status:    QuestStatusActive,
			SortOrder: 0,
		},
		{
			ID:        "daily_economy",
			Type:      QuestTypeDaily,
			Title:     "商人眼光",
			Desc:      "查看 1 次经济行情",
			Icon:      "📊",
			Goal:      1,
			Reward:    QuestReward{Diamond: 30, Gold: 500, Exp: 10, Icon: "💰"},
			Status:    QuestStatusActive,
			SortOrder: 1,
		},
		{
			ID:        "daily_gacha",
			Type:      QuestTypeDaily,
			Title:     "召唤之心",
			Desc:      "进行 1 次抽卡",
			Icon:      "✨",
			Goal:      1,
			Reward:    QuestReward{Diamond: 20, Gold: 300, Exp: 15, Icon: "🎴"},
			Status:    QuestStatusActive,
			SortOrder: 2,
		},
		// 成就任务
		{
			ID:        "achieve_tick_100",
			Type:      QuestTypeAchieve,
			Title:     "时间见证者",
			Desc:      "世界运行达到 100 tick",
			Icon:      "⏳",
			Goal:      100,
			Reward:    QuestReward{Diamond: 200, Gold: 5000, Exp: 100, Icon: "🏆"},
			Status:    QuestStatusActive,
			SortOrder: 10,
		},
		{
			ID:        "achieve_tick_1000",
			Type:      QuestTypeAchieve,
			Title:     "世纪旁观者",
			Desc:      "世界运行达到 1000 tick",
			Icon:      "🌟",
			Goal:      1000,
			Reward:    QuestReward{Diamond: 500, Gold: 20000, Exp: 500, Icon: "👑"},
			Status:    QuestStatusLocked,
			SortOrder: 11,
		},
		{
			ID:        "achieve_npc_50",
			Type:      QuestTypeAchieve,
			Title:     "人口普查员",
			Desc:      "查看 50 个 NPC",
			Icon:      "👥",
			Goal:      50,
			Reward:    QuestReward{Diamond: 100, Gold: 3000, Exp: 50, Icon: "📋"},
			Status:    QuestStatusActive,
			SortOrder: 12,
		},
		// 经济任务
		{
			ID:        "econ_trade_5",
			Type:      QuestTypeEconomy,
			Title:     "初涉商道",
			Desc:      "查看 5 次经济行情",
			Icon:      "📈",
			Goal:      5,
			Reward:    QuestReward{Diamond: 80, Gold: 2000, Exp: 40, Icon: "💹"},
			Status:    QuestStatusActive,
			SortOrder: 20,
		},
		// 故事任务
		{
			ID:        "story_first_look",
			Type:      QuestTypeStory,
			Title:     "新世界的第一眼",
			Desc:      "进入世界并观察 NPC",
			Icon:      "📖",
			Goal:      1,
			Reward:    QuestReward{Diamond: 100, Gold: 2000, Exp: 50, Icon: "🎁"},
			Status:    QuestStatusActive,
			SortOrder: 30,
		},
	}

	for _, q := range quests {
		qm.quests[q.ID] = q
	}
}

func (qm *QuestManager) GetQuests() []Quest {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	result := make([]Quest, 0, len(qm.quests))
	for _, q := range qm.quests {
		result = append(result, *q)
	}
	return result
}

func (qm *QuestManager) GetQuestsByType(qt QuestType) []Quest {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	result := make([]Quest, 0)
	for _, q := range qm.quests {
		if q.Type == qt {
			result = append(result, *q)
		}
	}
	return result
}

func (qm *QuestManager) UpdateProgress(questID string, progress int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	q, ok := qm.quests[questID]
	if !ok || q.Status != QuestStatusActive {
		return
	}

	if progress > q.Goal {
		progress = q.Goal
	}
	q.Progress = progress
}

func (qm *QuestManager) AddProgress(questID string, delta int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	q, ok := qm.quests[questID]
	if !ok || q.Status != QuestStatusActive {
		return
	}

	q.Progress += delta
	if q.Progress > q.Goal {
		q.Progress = q.Goal
	}
}

func (qm *QuestManager) ClaimReward(questID string) (*QuestReward, bool) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	q, ok := qm.quests[questID]
	if !ok || q.Status != QuestStatusActive || q.Progress < q.Goal {
		return nil, false
	}

	q.Status = QuestStatusClaimed
	reward := q.Reward

	// 解锁后续任务
	qm.unlockChain(questID)

	return &reward, true
}

func (qm *QuestManager) unlockChain(completedID string) {
	chain := map[string]string{
		"achieve_tick_100":  "achieve_tick_1000",
	}

	if nextID, ok := chain[completedID]; ok {
		if next, exists := qm.quests[nextID]; exists && next.Status == QuestStatusLocked {
			next.Status = QuestStatusActive
		}
	}
}

func (qm *QuestManager) RecordNPCView() {
	qm.AddProgress("daily_observe_10", 1)
	qm.AddProgress("achieve_npc_50", 1)
	qm.AddProgress("story_first_look", 1)
	qm.mu.Lock()
	qm.playerStats.TotalNPCsChecked++
	qm.mu.Unlock()
}

func (qm *QuestManager) RecordEconomyView() {
	qm.AddProgress("daily_economy", 1)
	qm.AddProgress("econ_trade_5", 1)
	qm.mu.Lock()
	qm.playerStats.TotalEconomyChecks++
	qm.mu.Unlock()
}

func (qm *QuestManager) RecordGacha() {
	qm.AddProgress("daily_gacha", 1)
	qm.mu.Lock()
	qm.playerStats.TotalGachaPulls++
	qm.mu.Unlock()
}

func (qm *QuestManager) UpdateWorldTick(tick int) {
	qm.UpdateProgress("achieve_tick_100", tick)
	qm.UpdateProgress("achieve_tick_1000", tick)
	qm.mu.Lock()
	qm.playerStats.WorldTicksSeen = tick
	qm.mu.Unlock()
}

func (qm *QuestManager) GetPlayerStats() PlayerStats {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return qm.playerStats
}

func (qm *QuestManager) GetQuestSummary() map[string]interface{} {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	quests := make([]Quest, 0, len(qm.quests))
	for _, q := range qm.quests {
		quests = append(quests, *q)
	}

	var activeCount, claimedCount, completableCount int
	for _, q := range quests {
		switch q.Status {
		case QuestStatusActive:
			activeCount++
			if q.Progress >= q.Goal {
				completableCount++
			}
		case QuestStatusClaimed:
			claimedCount++
		}
	}

	return map[string]interface{}{
		"quests":          quests,
		"active_count":    activeCount,
		"claimed_count":   claimedCount,
		"completable":     completableCount,
		"player_stats":    qm.playerStats,
	}
}
