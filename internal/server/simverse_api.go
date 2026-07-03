package server

import (
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Soltus/encv-go/internal/simverse"
	"github.com/gin-gonic/gin"
)

type SimverseManager struct {
	mu     sync.RWMutex
	world  *simverse.FractalWorld
	rng    *rand.Rand
	running bool
	stopCh chan struct{}
	doneCh chan struct{}

	tickHistory []worldTickSample
}

type worldTickSample struct {
	timestamp time.Time
	tick      uint32
	duration  time.Duration
}

func NewSimverseManager() *SimverseManager {
	world := simverse.NewFractalWorld()
	world.SetPerformanceTier(simverse.PerfTierBackground)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < 50; i++ {
		world.AddFocusNPC(uint64(i), simverse.FocusCore)
		world.GetBrain(uint64(i))
	}
	for i := 0; i < 2000; i++ {
		world.GetNPC(uint64(i), rng)
	}

	return &SimverseManager{
		world:       world,
		rng:         rng,
		running:     false,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		tickHistory: make([]worldTickSample, 0, 1000),
	}
}

func (sm *SimverseManager) Start() {
	sm.mu.Lock()
	if sm.running {
		sm.mu.Unlock()
		return
	}
	sm.running = true
	sm.stopCh = make(chan struct{})
	sm.doneCh = make(chan struct{})
	sm.mu.Unlock()

	go sm.runLoop()
}

func (sm *SimverseManager) Stop() {
	sm.mu.Lock()
	if !sm.running {
		sm.mu.Unlock()
		return
	}
	sm.running = false
	close(sm.stopCh)
	sm.mu.Unlock()

	<-sm.doneCh
}

func (sm *SimverseManager) runLoop() {
	defer close(sm.doneCh)

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopCh:
			return
		case <-ticker.C:
			sm.tickOnce()
		}
	}
}

func (sm *SimverseManager) tickOnce() {
	sm.mu.Lock()
	if !sm.running {
		sm.mu.Unlock()
		return
	}
	start := time.Now()

	sm.world.Tick(sm.rng)

	dur := time.Since(start)
	currentTick := sm.world.WorldTick()

	sm.tickHistory = append(sm.tickHistory, worldTickSample{
		timestamp: time.Now(),
		tick:      currentTick,
		duration:  dur,
	})
	if len(sm.tickHistory) > 1000 {
		sm.tickHistory = sm.tickHistory[1:]
	}
	sm.mu.Unlock()
}

func (sm *SimverseManager) World() *simverse.FractalWorld {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.world
}

func (sm *SimverseManager) IsRunning() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.running
}

func (sm *SimverseManager) SetTier(tier simverse.PerformanceTier) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.world.SetPerformanceTier(tier)
}

func (s *Server) handleSimverseWorldState(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	stats := world.MemoryStats()
	tier := world.PerfTierName()

	c.JSON(http.StatusOK, gin.H{
		"tick":      world.WorldTick(),
		"tier":      tier,
		"running":   s.simverseMgr.IsRunning(),
		"npc_count": stats["npc_cache_count"],
		"npc_mb":    stats["npc_cache_mb"],
		"brain_count": stats["brain_cache_count"],
		"brain_mb":  stats["brain_cache_mb"],
		"cell_count": stats["cell_cache_count"],
		"cell_mb":   stats["cell_cache_mb"],
		"focus_count": stats["focus_npc_count"],
		"total_mb":  stats["total_mb"],
	})
}

func (s *Server) handleSimverseWorldConfig(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	tier := world.CurrentTier()
	config := simverse.PerfTiers[tier]

	c.JSON(http.StatusOK, gin.H{
		"tier":           tier,
		"tier_name":      world.PerfTierName(),
		"event_rate_mul": config.EventRateMul,
		"cache_size":     config.CacheSize,
		"sub_sim_active": config.SubSimActive,
		"sub_sim_depth":  config.SubSimDepth,
	})
}

func (s *Server) handleSimverseSetConfig(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	var req struct {
		Tier string `json:"tier"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var tier simverse.PerformanceTier
	switch req.Tier {
	case "background":
		tier = simverse.PerfTierBackground
	case "foreground":
		tier = simverse.PerfTierForeground
	case "fg_idle":
		tier = simverse.PerfTierFgIdle
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tier, must be background/foreground/fg_idle"})
		return
	}

	s.simverseMgr.SetTier(tier)

	world := s.simverseMgr.World()
	config := simverse.PerfTiers[tier]
	c.JSON(http.StatusOK, gin.H{
		"tier":           tier,
		"tier_name":      world.PerfTierName(),
		"event_rate_mul": config.EventRateMul,
		"cache_size":     config.CacheSize,
		"sub_sim_active": config.SubSimActive,
		"sub_sim_depth":  config.SubSimDepth,
	})
}

func (s *Server) handleSimverseWorldControl(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	var req struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch req.Action {
	case "start":
		s.simverseMgr.Start()
		c.JSON(http.StatusOK, gin.H{"status": "started"})
	case "stop":
		s.simverseMgr.Stop()
		c.JSON(http.StatusOK, gin.H{"status": "stopped"})
	case "pause":
		s.simverseMgr.Stop()
		c.JSON(http.StatusOK, gin.H{"status": "paused"})
	case "resume":
		s.simverseMgr.Start()
		c.JSON(http.StatusOK, gin.H{"status": "resumed"})
	case "step":
		s.simverseMgr.tickOnce()
		world := s.simverseMgr.World()
		c.JSON(http.StatusOK, gin.H{"status": "stepped", "tick": world.WorldTick()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action, must be start/stop/pause/resume/step"})
	}
}

func (s *Server) handleSimverseNPCList(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	world := s.simverseMgr.World()
	stats := world.MemoryStats()
	total := int(stats["npc_cache_count"])

	startID := (page - 1) * pageSize
	npcs := make([]gin.H, 0, pageSize)

	for i := 0; i < pageSize; i++ {
		id := uint64(startID + i)
		npc := world.GetNPC(id, s.simverseMgr.rng)
		npcs = append(npcs, gin.H{
			"id":       npc.ID,
			"name":     npc.Name,
			"species":  npc.Species.String(),
			"gender":   npc.Gender.String(),
			"age":      npc.Age,
			"stage":    npc.Stage.String(),
			"profession": npc.Profession.String(),
			"health":   npc.Health,
			"energy":   npc.Energy,
			"charisma": npc.Charisma,
			"intelligence": npc.Intelligence,
			"strength": npc.Strength,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     npcs,
	})
}

func (s *Server) handleSimverseNPCDetail(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	world := s.simverseMgr.World()
	npc := world.GetNPC(id, s.simverseMgr.rng)

	skills := make(map[string]uint16)
	for k, v := range npc.Skills {
		skills[k.String()] = v
	}

	resources := make(map[string]int64)
	for k, v := range npc.Resources {
		resources[k.String()] = v
	}

	c.JSON(http.StatusOK, gin.H{
		"id":               npc.ID,
		"name":             npc.Name,
		"species":          npc.Species.String(),
		"gender":           npc.Gender.String(),
		"gender_identity":  npc.GenderIdentity.String(),
		"sexual_orient":    npc.SexualOrient.String(),
		"age":              npc.Age,
		"stage":            npc.Stage.String(),
		"profession":       npc.Profession.String(),
		"health":           npc.Health,
		"max_health":       npc.MaxHealth,
		"energy":           npc.Energy,
		"max_energy":       npc.MaxEnergy,
		"charisma":         npc.Charisma,
		"intelligence":     npc.Intelligence,
		"strength":         npc.Strength,
		"luck":             npc.Luck,
		"wealth":           npc.Wealth,
		"reputation":       npc.Reputation,
		"skills":           skills,
		"resources":        resources,
		"life_events":      npc.LifeEvents,
		"last_tick":        npc.LastTick,
		"birth_tick":       npc.BirthTick,
		"is_alive":         npc.IsAlive,
	})
}

func (s *Server) handleSimversePerfMetrics(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	s.simverseMgr.mu.RLock()
	history := s.simverseMgr.tickHistory
	s.simverseMgr.mu.RUnlock()

	var avgTickNs int64
	var minTickNs int64 = 1e9
	var maxTickNs int64
	var ticksPerSec float64

	if len(history) > 0 {
		sum := int64(0)
		for _, s := range history {
			ns := s.duration.Nanoseconds()
			sum += ns
			if ns < minTickNs {
				minTickNs = ns
			}
			if ns > maxTickNs {
				maxTickNs = ns
			}
		}
		avgTickNs = sum / int64(len(history))

		first := history[0]
		last := history[len(history)-1]
		elapsed := last.timestamp.Sub(first.timestamp).Seconds()
		tickDiff := last.tick - first.tick
		if elapsed > 0 {
			ticksPerSec = float64(tickDiff) / elapsed
		}
	}

	world := s.simverseMgr.World()
	stats := world.MemoryStats()

	c.JSON(http.StatusOK, gin.H{
		"avg_tick_ns":  avgTickNs,
		"min_tick_ns":  minTickNs,
		"max_tick_ns":  maxTickNs,
		"ticks_per_sec": ticksPerSec,
		"samples":      len(history),
		"npc_count":    stats["npc_cache_count"],
		"npc_mb":       stats["npc_cache_mb"],
		"total_mb":     stats["total_mb"],
		"running":      s.simverseMgr.IsRunning(),
		"tier":         world.PerfTierName(),
	})
}

func (s *Server) handleSimverseFocusList(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	focusIDs := world.ListFocusNPCs()

	npcs := make([]gin.H, 0, len(focusIDs))
	for _, id := range focusIDs {
		npc := world.GetNPC(id, s.simverseMgr.rng)
		level := world.FocusLevel(id)
		npcs = append(npcs, gin.H{
			"id":    npc.ID,
			"name":  npc.Name,
			"level": level.String(),
			"stage": npc.Stage.String(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"count": len(npcs),
		"items": npcs,
	})
}

func (s *Server) handleSimverseSetFocus(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	var req struct {
		NPCs []struct {
			ID    uint64 `json:"id"`
			Level string `json:"level"`
		} `json:"npcs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	world := s.simverseMgr.World()

	for _, n := range req.NPCs {
		var level simverse.FocusLevel
		switch n.Level {
		case "player":
			level = simverse.FocusPlayer
		case "core":
			level = simverse.FocusCore
		case "near":
			level = simverse.FocusNear
		case "distant":
			level = simverse.FocusDistant
		case "none":
			world.RemoveFocusNPC(n.ID)
			continue
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid focus level: " + n.Level})
			return
		}
		world.AddFocusNPC(n.ID, level)
	}

	focusIDs := world.ListFocusNPCs()
	c.JSON(http.StatusOK, gin.H{
		"count": len(focusIDs),
		"items": focusIDs,
	})
}
