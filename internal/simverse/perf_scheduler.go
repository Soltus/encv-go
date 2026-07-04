package simverse

type PerformanceTier int

const (
	PerfTierBackground   PerformanceTier = 0
	PerfTierForeground   PerformanceTier = 1
	PerfTierFgIdle       PerformanceTier = 2
)

type PerfTierConfig struct {
	Tier           PerformanceTier
	Name           string
	EventRateMul   float64
	CatchUpBatch   int
	CacheSize      int
	DetailLevel    int
	SubSimActive   bool
	SubSimDepth    int
	APIBurstLimit  int
}

var PerfTiers = [...]PerfTierConfig{
	{
		Tier:         PerfTierBackground,
		Name:         "background",
		EventRateMul: 0.6,
		CatchUpBatch: 100,
		CacheSize:    5000,
		DetailLevel:  1,
		SubSimActive: false,
		SubSimDepth:  0,
		APIBurstLimit: 10,
	},
	{
		Tier:         PerfTierForeground,
		Name:         "foreground",
		EventRateMul: 1.0,
		CatchUpBatch: 500,
		CacheSize:    15000,
		DetailLevel:  3,
		SubSimActive: true,
		SubSimDepth:  2,
		APIBurstLimit: 100,
	},
	{
		Tier:         PerfTierFgIdle,
		Name:         "foreground_idle",
		EventRateMul: 3.0,
		CatchUpBatch: 2000,
		CacheSize:    30000,
		DetailLevel:  2,
		SubSimActive: true,
		SubSimDepth:  3,
		APIBurstLimit: 50,
	},
}

type PerfScheduler struct {
	currentTier   PerformanceTier
	lastChangeTick uint32
	focusNPCs     map[uint64]uint8
}

func NewPerfScheduler() *PerfScheduler {
	return &PerfScheduler{
		currentTier: PerfTierForeground,
		focusNPCs:   make(map[uint64]uint8),
	}
}

func (p *PerfScheduler) CurrentTier() PerformanceTier {
	return p.currentTier
}

func (p *PerfScheduler) SetTier(tier PerformanceTier) {
	p.currentTier = tier
}

func (p *PerfScheduler) Config() PerfTierConfig {
	return PerfTiers[p.currentTier]
}

func (p *PerfScheduler) AddFocusNPC(npcID uint64, priority uint8) {
	if existing, ok := p.focusNPCs[npcID]; !ok || priority > existing {
		p.focusNPCs[npcID] = priority
	}
}

func (p *PerfScheduler) RemoveFocusNPC(npcID uint64) {
	delete(p.focusNPCs, npcID)
}

func (p *PerfScheduler) FocusCount() int {
	return len(p.focusNPCs)
}

func (p *PerfScheduler) DetailLevelForNPC(npcID uint64) int {
	priority, focused := p.focusNPCs[npcID]
	baseDetail := p.Config().DetailLevel
	if focused {
		return baseDetail + int(priority)
	}
	return baseDetail
}
