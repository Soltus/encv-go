package server

import (
	"log/slog"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Soltus/encv-go/internal/simverse"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type SimverseManager struct {
	mu     sync.RWMutex
	world  *simverse.FractalWorld
	rng    *rand.Rand
	running bool
	stopCh chan struct{}
	doneCh chan struct{}

	tickHistory []worldTickSample

	wsClients   map[*simverseWSClient]bool
	wsBroadcast chan simverseWSMessage
	wsUpgrader  websocket.Upgrader

	dataDir  string
	lastCheckpointTick uint32
	checkpointInterval uint32
}

type worldTickSample struct {
	timestamp time.Time
	tick      uint32
	duration  time.Duration
}

func NewSimverseManager(dataDir string) *SimverseManager {
	world := simverse.NewFractalWorld(dataDir, "default")
	world.SetPerformanceTier(simverse.PerfTierBackground)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	loaded, _ := world.LoadCheckpoint()
	if !loaded {
		for i := 0; i < 50; i++ {
			world.AddFocusNPC(uint64(i), simverse.FocusCore)
			world.GetBrain(uint64(i))
		}
		for i := 0; i < 2000; i++ {
			world.GetNPC(uint64(i), rng)
		}
	}

	mgr := &SimverseManager{
		world:       world,
		rng:         rng,
		running:     false,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		tickHistory: make([]worldTickSample, 0, 1000),
		wsClients:   make(map[*simverseWSClient]bool),
		wsBroadcast: make(chan simverseWSMessage, 256),
		dataDir:     dataDir,
		checkpointInterval: 500,
		wsUpgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	go func() {
		if err := world.Persistence().CreatePlaceholder(); err != nil {
			slog.Warn("simverse placeholder creation failed", "error", err)
		} else if world.Persistence().HasPlaceholder() {
			slog.Info("simverse storage placeholder created",
				"size_bytes", world.Persistence().PlaceholderSize())
		}
	}()

	return mgr
}

func (sm *SimverseManager) SaveCheckpoint() error {
	return sm.world.SaveCheckpoint()
}

func (sm *SimverseManager) HasCheckpoint() bool {
	return sm.world.HasCheckpoint()
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

	if err := sm.world.SaveCheckpoint(); err != nil {
		slog.Warn("simverse final checkpoint failed", "error", err)
	}
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

	needsCheckpoint := currentTick-sm.lastCheckpointTick >= sm.checkpointInterval
	if needsCheckpoint {
		sm.lastCheckpointTick = currentTick
		go func() {
			if err := sm.world.SaveCheckpoint(); err != nil {
				slog.Warn("simverse checkpoint failed", "error", err, "tick", currentTick)
			}
			persist := sm.world.Persistence()
			if action := persist.CheckStorageAndAdjust(sm.world); action != "normal" {
				slog.Warn("simverse storage adjustment", "action", action, "tick", currentTick)
			}
		}()
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
		bs := world.GetBehaviorState(id, s.simverseMgr.rng)
		npcs = append(npcs, gin.H{
			"id":              npc.ID,
			"name":            npc.Name,
			"species":         npc.Species.String(),
			"gender":          npc.Gender.String(),
			"age":             npc.Age,
			"life_stage":      npc.LifeStage.String(),
			"profession":      npc.Profession.String(),
			"level":           npc.Level,
			"health":          npc.Health,
			"max_health":      npc.MaxHealth,
			"energy":          npc.Energy,
			"max_energy":      npc.MaxEnergy,
			"is_alive":        npc.IsAlive,
			"wealth_tier":     npc.WealthTier,
			"social_tier":     npc.SocialTier,
			"current_behavior": bs.CurrentBehavior.String(),
			"current_behavior_cn": bs.CurrentBehavior.CN(),
			"mood":            npc.Mood,
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
	behaviorState := world.GetBehaviorState(id, s.simverseMgr.rng)

	skills := make(map[string]uint8)
	for i := simverse.SkillType(0); i < simverse.SkillMax; i++ {
		skills[i.String()] = npc.Skills[i]
	}

	inventory := make(map[string]uint32)
	for i := simverse.ResourceType(0); i < simverse.ResMax; i++ {
		inventory[i.String()] = npc.Inventory[i]
	}

	bank := make(map[string]uint32)
	for i := simverse.ResourceType(0); i < simverse.ResMax; i++ {
		bank[i.String()] = npc.Bank[i]
	}

	bigFive := make(map[string]uint8)
	for i := simverse.BigFiveTrait(0); i < simverse.BigFiveMax; i++ {
		bigFive[i.String()] = npc.Personality.BigFive[i]
	}

	values := make(map[string]uint8)
	for i := simverse.ValueType(0); i < simverse.ValueMax; i++ {
		values[i.String()] = npc.Personality.Values[i]
	}

	interests := make(map[string]uint8)
	for i := simverse.InterestType(0); i < simverse.InterestMax; i++ {
		interests[i.String()] = npc.Personality.Interests[i]
	}

	topValues := npc.Personality.Values.Top3()
	topValueNames := make([]string, len(topValues))
	for i, v := range topValues {
		topValueNames[i] = v.String()
	}

	topInterests := npc.Personality.Interests.TopN(5)
	topInterestNames := make([]string, len(topInterests))
	for i, v := range topInterests {
		topInterestNames[i] = v.String()
	}

	stmItems := npc.ShortTermMem.Items()
	shortTermMem := make([]gin.H, 0, len(stmItems))
	for _, m := range stmItems {
		shortTermMem = append(shortTermMem, gin.H{
			"id":          m.ID,
			"type":        m.Type.String(),
			"importance":  m.Importance,
			"target_id":   m.TargetID,
			"content_tag": m.ContentTag,
			"emotion_tag": m.EmotionTag,
			"created_at":  m.CreatedTick,
			"strength":    m.Strength,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"id":              npc.ID,
		"name":            npc.Name,
		"species":         npc.Species.String(),
		"gender":          npc.Gender.String(),
		"gender_identity": npc.GenderIdentity.String(),
		"sexual_orient":   npc.SexualOrient.String(),
		"age":             npc.Age,
		"life_stage":      npc.LifeStage.String(),
		"profession":      npc.Profession.String(),
		"level":           npc.Level,
		"career_stage":    npc.CareerStage,
		"health":          npc.Health,
		"max_health":      npc.MaxHealth,
		"energy":          npc.Energy,
		"max_energy":      npc.MaxEnergy,
		"mana":            npc.Mana,
		"max_mana":        npc.MaxMana,
		"mood":            npc.Mood,
		"satisfaction":    npc.Satisfaction,
		"wealth_tier":     npc.WealthTier,
		"social_tier":     npc.SocialTier,
		"experience":      npc.Experience,
		"num_children":    npc.NumChildren,
		"num_marriages":   npc.NumMarriages,
		"org_id":          npc.OrgID,
		"region_id":       npc.RegionID,
		"home_region_id":  npc.HomeRegionID,
		"behavior": gin.H{
			"current_behavior":  behaviorState.CurrentBehavior.String(),
			"current_behavior_cn": behaviorState.CurrentBehavior.CN(),
			"behavior_start_tick": behaviorState.BehaviorStartTick,
			"behavior_duration":  behaviorState.BehaviorDuration,
			"target_id":          behaviorState.TargetID,
			"target_pos_x":       behaviorState.TargetPosX,
			"target_pos_y":       behaviorState.TargetPosY,
			"needs": gin.H{
				"hunger":       behaviorState.Needs.Hunger,
				"energy":       behaviorState.Needs.Energy,
				"social":       behaviorState.Needs.Social,
				"achievement":  behaviorState.Needs.Achievement,
			},
		},
		"skills":          skills,
		"inventory":       inventory,
		"bank":            bank,
		"big_five":        bigFive,
		"values":          values,
		"interests":       interests,
		"top_values":      topValueNames,
		"top_interests":   topInterestNames,
		"short_term_mem":  shortTermMem,
		"life_events":     npc.LifeEvents,
		"born_at":         npc.BornAt,
		"died_at":         npc.DiedAt,
		"last_update":     npc.LastUpdateTick,
		"is_alive":        npc.IsAlive,
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
			"id":         npc.ID,
			"name":       npc.Name,
			"level":      level.String(),
			"life_stage": npc.LifeStage.String(),
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

type simverseWSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type simverseWSClient struct {
	conn  *websocket.Conn
	send  chan simverseWSMessage
	mgr   *SimverseManager
}

func (sm *SimverseManager) wsBroadcastLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	statsTicker := time.NewTicker(1 * time.Second)
	defer statsTicker.Stop()

	for {
		select {
		case msg, ok := <-sm.wsBroadcast:
			if !ok {
				return
			}
			sm.mu.RLock()
			clients := make([]*simverseWSClient, 0, len(sm.wsClients))
			for c := range sm.wsClients {
				clients = append(clients, c)
			}
			sm.mu.RUnlock()

			for _, c := range clients {
				select {
				case c.send <- msg:
				default:
				}
			}
		case <-ticker.C:
			sm.mu.RLock()
			running := sm.running
			sm.mu.RUnlock()
			if !running {
				continue
			}
			world := sm.World()
			sm.wsBroadcast <- simverseWSMessage{
				Type: "world:tick",
				Data: gin.H{
					"tick": world.WorldTick(),
					"tier": world.PerfTierName(),
				},
			}
		case <-statsTicker.C:
			sm.mu.RLock()
			running := sm.running
			sm.mu.RUnlock()
			if !running {
				continue
			}
			world := sm.World()
			stats := world.MemoryStats()
			sm.wsBroadcast <- simverseWSMessage{
				Type: "world:stats",
				Data: gin.H{
					"npc_count":   stats["npc_cache_count"],
					"npc_mb":      stats["npc_cache_mb"],
					"brain_count": stats["brain_cache_count"],
					"cell_count":  stats["cell_cache_count"],
					"total_mb":    stats["total_mb"],
					"focus_count": stats["focus_npc_count"],
				},
			}
		}
	}
}

func (sm *SimverseManager) RegisterWS(conn *websocket.Conn) *simverseWSClient {
	client := &simverseWSClient{
		conn: conn,
		send: make(chan simverseWSMessage, 64),
		mgr:  sm,
	}

	sm.mu.Lock()
	sm.wsClients[client] = true
	if len(sm.wsClients) == 1 {
		go sm.wsBroadcastLoop()
	}
	sm.mu.Unlock()

	slog.Info("simverse WS client connected", "total_clients", len(sm.wsClients))
	return client
}

func (sm *SimverseManager) UnregisterWS(client *simverseWSClient) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.wsClients[client]; ok {
		delete(sm.wsClients, client)
		close(client.send)
		slog.Info("simverse WS client disconnected", "total_clients", len(sm.wsClients))
	}
}

func (c *simverseWSClient) WritePump() {
	defer func() {
		c.mgr.UnregisterWS(c)
		c.conn.Close()
	}()

	for msg := range c.send {
		c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := c.conn.WriteJSON(msg); err != nil {
			slog.Debug("simverse WS write error", "error", err)
			return
		}
	}
}

func (c *simverseWSClient) ReadPump() {
	defer func() {
		c.mgr.UnregisterWS(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var msg map[string]interface{}
		if err := c.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Debug("simverse WS read error", "error", err)
			}
			break
		}

		msgType, _ := msg["type"].(string)
		switch msgType {
		case "ping":
			select {
			case c.send <- simverseWSMessage{Type: "pong", Data: gin.H{"ts": time.Now().Unix()}}:
			default:
			}
		}
	}
}

func (s *Server) handleSimverseWebSocket(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	conn, err := s.simverseMgr.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		slog.Error("simverse WS upgrade failed", "error", err)
		return
	}

	client := s.simverseMgr.RegisterWS(conn)

	go client.WritePump()
	client.ReadPump()
}

func chronicleEventToJSON(evt simverse.ChronicleEvent) gin.H {
	return gin.H{
		"id":          evt.ID,
		"tick":        evt.Tick,
		"level":       evt.Level.String(),
		"level_cn":    evt.Level.CN(),
		"type":        evt.Type.String(),
		"type_cn":     evt.Type.CN(),
		"importance":  evt.Importance,
		"imp_name":    evt.Importance.String(),
		"imp_cn":      evt.Importance.CN(),
		"entity_id":   evt.EntityID,
		"target_id":   evt.TargetID,
		"data_tag":    evt.DataTag,
		"cause1_id":   evt.Cause1ID,
		"cause2_id":   evt.Cause2ID,
		"cause3_id":   evt.Cause3ID,
	}
}

func (s *Server) handleSimverseChronicleWorld(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	chron := world.Chronicle()

	minImp := 0
	if v := c.Query("min_importance"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n < int(simverse.ImpMax) {
			minImp = n
		}
	}

	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	events := chron.WorldTimeline(simverse.ChronicleImportance(minImp), limit)

	result := make([]gin.H, 0, len(events))
	for _, evt := range events {
		result = append(result, chronicleEventToJSON(evt))
	}

	c.JSON(http.StatusOK, gin.H{
		"count":    len(result),
		"era":      chron.CurrentEra(),
		"total_events": chron.Count(),
		"items":    result,
	})
}

func (s *Server) handleSimverseChronicleNPC(c *gin.Context) {
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
	chron := world.Chronicle()

	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	events := chron.NPCHistory(id, limit)

	result := make([]gin.H, 0, len(events))
	for _, evt := range events {
		result = append(result, chronicleEventToJSON(evt))
	}

	c.JSON(http.StatusOK, gin.H{
		"npc_id": id,
		"count":  len(result),
		"items":  result,
	})
}

func (s *Server) handleSimverseChronicleEvent(c *gin.Context) {
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
	chron := world.Chronicle()

	evt, ok := chron.GetEvent(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
		return
	}

	causes := chron.Causes(id)
	causesJSON := make([]gin.H, 0, len(causes))
	for _, c := range causes {
		causesJSON = append(causesJSON, chronicleEventToJSON(c))
	}

	effects := chron.Effects(id, 10)
	effectsJSON := make([]gin.H, 0, len(effects))
	for _, e := range effects {
		effectsJSON = append(effectsJSON, chronicleEventToJSON(e))
	}

	result := chronicleEventToJSON(evt)
	result["causes"] = causesJSON
	result["effects"] = effectsJSON

	c.JSON(http.StatusOK, result)
}

func (s *Server) handleSimverseWorldSave(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	if err := s.simverseMgr.SaveCheckpoint(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	world := s.simverseMgr.World()
	meta := world.Persistence().Metadata()
	c.JSON(http.StatusOK, gin.H{
		"status":    "saved",
		"tick":      meta.Tick,
		"era":       meta.CurrentEra,
		"saved_at":  meta.LastSavedAt,
	})
}

func (s *Server) handleSimverseWorldLoad(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	wasRunning := s.simverseMgr.IsRunning()
	if wasRunning {
		s.simverseMgr.Stop()
	}

	loaded, err := s.simverseMgr.World().LoadCheckpoint()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !loaded {
		c.JSON(http.StatusNotFound, gin.H{"error": "no save found"})
		return
	}

	if wasRunning {
		s.simverseMgr.Start()
	}

	world := s.simverseMgr.World()
	meta := world.Persistence().Metadata()
	c.JSON(http.StatusOK, gin.H{
		"status":   "loaded",
		"tick":     meta.Tick,
		"era":      meta.CurrentEra,
		"running":  wasRunning,
	})
}

func (s *Server) handleSimverseWorldSaveInfo(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	persist := world.Persistence()
	meta := persist.Metadata()

	c.JSON(http.StatusOK, gin.H{
		"has_save":   persist.HasExistingSave(),
		"tick":       meta.Tick,
		"era":        meta.CurrentEra,
		"npc_count":  meta.NPCCount,
		"focus_count": meta.FocusCount,
		"perf_tier":  meta.PerfTier,
		"saved_at":   meta.LastSavedAt,
		"created_at": meta.CreatedAt,
		"save_state": meta.SaveState,
		"world_id":   meta.WorldID,
		"world_name": meta.WorldName,
	})
}

func (s *Server) handleSimverseStorageStatus(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	status := world.Persistence().GetStorageStatus()

	c.JSON(http.StatusOK, gin.H{
		"level":              status.Level,
		"total_bytes":        status.TotalBytes,
		"available_bytes":    status.AvailableBytes,
		"used_bytes":         status.UsedBytes,
		"placeholder_active": status.PlaceholderActive,
		"placeholder_size":   status.PlaceholderSize,
		"low_threshold":      simverse.StorageLowBytes,
		"critical_threshold": simverse.StorageCriticalBytes,
	})
}

func (s *Server) handleSimverseBehaviorStats(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	stats := world.MemoryStats()
	total := int(stats["npc_cache_count"])
	if total > 500 {
		total = 500
	}

	behaviorCounts := make(map[string]int)
	aliveCount := 0

	for i := 0; i < total; i++ {
		id := uint64(i)
		npc := world.GetNPC(id, s.simverseMgr.rng)
		if !npc.IsAlive {
			continue
		}
		aliveCount++
		bs := world.GetBehaviorState(id, s.simverseMgr.rng)
		behaviorCounts[bs.CurrentBehavior.String()]++
	}

	c.JSON(http.StatusOK, gin.H{
		"total_npcs":   total,
		"alive_npcs":   aliveCount,
		"behavior_dist": behaviorCounts,
	})
}

func (s *Server) handleSimverseBehaviorList(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	behaviorFilter := c.DefaultQuery("behavior", "")

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
	items := make([]gin.H, 0, pageSize)
	filteredTotal := 0

	for i := 0; i < total; i++ {
		id := uint64(i)
		npc := world.GetNPC(id, s.simverseMgr.rng)
		if !npc.IsAlive {
			continue
		}

		bs := world.GetBehaviorState(id, s.simverseMgr.rng)

		if behaviorFilter != "" && bs.CurrentBehavior.String() != behaviorFilter {
			continue
		}

		filteredTotal++

		if i < startID || i >= startID+pageSize {
			continue
		}

		items = append(items, gin.H{
			"npc_id":              npc.ID,
			"npc_name":            npc.Name,
			"profession":          npc.Profession.String(),
			"level":               npc.Level,
			"current_behavior":    bs.CurrentBehavior.String(),
			"current_behavior_cn": bs.CurrentBehavior.CN(),
			"behavior_start_tick": bs.BehaviorStartTick,
			"behavior_duration":   bs.BehaviorDuration,
			"mood":                npc.Mood,
			"energy":              npc.Energy,
			"needs": gin.H{
				"hunger":      bs.Needs.Hunger,
				"energy":      bs.Needs.Energy,
				"social":      bs.Needs.Social,
				"achievement": bs.Needs.Achievement,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     filteredTotal,
		"items":     items,
	})
}

func (s *Server) handleSimverseEconomyStats(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	em := world.EconomyManager()
	regionID := uint32(1)
	stats := em.GetRegionalStats(regionID)

	c.JSON(http.StatusOK, stats)
}

func (s *Server) handleSimverseEconomyWealthRank(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	count := 20
	if v := c.Query("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			count = n
		}
	}

	em := world.EconomyManager()
	ranking := em.GetTopNPCsByWealth(world, count)

	c.JSON(http.StatusOK, gin.H{
		"count": len(ranking),
		"items": ranking,
	})
}

func (s *Server) handleSimverseQuestList(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	qm := world.QuestManager()
	summary := qm.GetQuestSummary()

	c.JSON(http.StatusOK, summary)
}

func (s *Server) handleSimverseQuestClaim(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	var req struct {
		QuestID string `json:"quest_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	qm := world.QuestManager()
	reward, ok := qm.ClaimReward(req.QuestID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quest not completable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"reward":  reward,
	})
}

func (s *Server) handleSimverseQuestAction(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}

	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	var req struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	qm := world.QuestManager()
	switch req.Action {
	case "view_npc":
		qm.RecordNPCView()
	case "view_economy":
		qm.RecordEconomyView()
	case "gacha":
		qm.RecordGacha()
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ============================================================
// 纪元 / 区域 / 组织 / 经济 聚合接口（Phase 后端补全）
// ============================================================

func npcBrief(npc *simverse.NPCV3) gin.H {
	return gin.H{
		"id":           npc.ID,
		"name":         npc.Name,
		"species":      npc.Species.String(),
		"gender":       npc.Gender.String(),
		"age":          npc.Age,
		"profession":   npc.Profession.String(),
		"level":        npc.Level,
		"org_id":       npc.OrgID,
		"region_id":    npc.RegionID,
		"wealth_tier":  npc.WealthTier,
		"career_stage": npc.CareerStage,
		"is_alive":     npc.IsAlive,
	}
}

// GET /api/simverse/era/current : 当前纪元 + 世界编年史概览
func (s *Server) handleSimverseEraCurrent(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	chron := world.Chronicle()
	era := chron.CurrentEra()

	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	events := chron.WorldTimeline(simverse.ImpMajor, limit)
	eventDTOs := make([]gin.H, 0, len(events))
	for _, e := range events {
		eventDTOs = append(eventDTOs, gin.H{
			"id":         e.ID,
			"tick":       e.Tick,
			"type":       e.Type.String(),
			"importance": e.Importance,
			"data_tag":   e.DataTag,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"era":          era,
		"world_tick":   world.WorldTick(),
		"event_count":  chron.Count(),
		"events":       eventDTOs,
	})
}

// GET /api/simverse/region/list : 区域聚合列表
func (s *Server) handleSimverseRegionList(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	aggs := world.GetRegionAggregates()
	c.JSON(http.StatusOK, gin.H{
		"count": len(aggs),
		"items": aggs,
	})
}

// GET /api/simverse/region/:id : 单个区域详情
func (s *Server) handleSimverseRegionDetail(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid region id"})
		return
	}

	agg, ok := world.GetRegionAggregate(uint32(id))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "region not found"})
		return
	}

	chron := world.Chronicle()
	regionEvents := chron.RegionHistory(uint32(id), 20)
	eventDTOs := make([]gin.H, 0, len(regionEvents))
	for _, e := range regionEvents {
		eventDTOs = append(eventDTOs, gin.H{
			"id":         e.ID,
			"tick":       e.Tick,
			"type":       e.Type.String(),
			"importance": e.Importance,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"region": agg,
		"events": eventDTOs,
	})
}

// GET /api/simverse/org/list : 组织聚合列表
func (s *Server) handleSimverseOrgList(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	aggs := world.GetOrgAggregates()
	c.JSON(http.StatusOK, gin.H{
		"count": len(aggs),
		"items": aggs,
	})
}

// GET /api/simverse/org/:id : 单个组织详情
func (s *Server) handleSimverseOrgDetail(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org id"})
		return
	}

	agg, ok := world.GetOrgAggregate(uint32(id))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "org not found"})
		return
	}

	c.JSON(http.StatusOK, agg)
}

// GET /api/simverse/org/:id/members : 组织成员（分页）
func (s *Server) handleSimverseOrgMembers(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org id"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	members, total := world.GetOrgMembers(uint32(id), page, pageSize)

	items := make([]gin.H, 0, len(members))
	for _, npc := range members {
		items = append(items, npcBrief(npc))
	}

	c.JSON(http.StatusOK, gin.H{
		"org_id":     uint32(id),
		"page":       page,
		"page_size":  pageSize,
		"total":      total,
		"count":      len(items),
		"items":      items,
	})
}

// GET /api/simverse/org/:id/territory : 组织领地（成员区域分布）
func (s *Server) handleSimverseOrgTerritory(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org id"})
		return
	}

	agg, ok := world.GetOrgAggregate(uint32(id))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "org not found"})
		return
	}

	// 领地按成员数量降序
	type terr struct {
		RegionID uint32 `json:"region_id"`
		Members  int    `json:"members"`
	}
	territory := make([]terr, 0, len(agg.RegionDist))
	for rid, cnt := range agg.RegionDist {
		territory = append(territory, terr{RegionID: rid, Members: cnt})
	}
	sort.Slice(territory, func(i, j int) bool {
		return territory[i].Members > territory[j].Members
	})

	c.JSON(http.StatusOK, gin.H{
		"org_id":    uint32(id),
		"name":      agg.Name,
		"territory": territory,
	})
}

// GET /api/simverse/social/stats?region=:id&org=:id : 社交关系统计
func (s *Server) handleSimverseSocialStats(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	var regionFilter, orgFilter uint32
	if v := c.Query("region"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			regionFilter = uint32(n)
		}
	}
	if v := c.Query("org"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			orgFilter = uint32(n)
		}
	}

	stats := world.GetSocialStats(regionFilter, orgFilter)
	c.JSON(http.StatusOK, gin.H{
		"sampled_npcs":   stats.SampledNPCs,
		"total_relations": stats.TotalRelations,
		"by_type":        stats.ByType,
		"by_region":      stats.ByRegion,
		"by_org":         stats.ByOrg,
	})
}

// GET /api/simverse/npc/:id/relations : 指定 NPC 的关系列表（含目标档案）
func (s *Server) handleSimverseNPCRelations(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid npc id"})
		return
	}

	npc := world.GetNPC(id, s.simverseMgr.rng)
	rels := world.GetNPCRelationships(id, s.simverseMgr.rng)

	byType := make(map[string]int)
	items := make([]gin.H, 0, len(rels))
	for _, r := range rels {
		byType[r.RelType.String()]++
		target := world.GetNPC(r.TargetID, s.simverseMgr.rng)
		items = append(items, gin.H{
			"target_id":   r.TargetID,
			"rel_type":    r.RelType.String(),
			"rel_type_id": int(r.RelType),
			"affinity":    r.Affinity,
			"last_meet":   r.LastMeet,
			"target":      npcBrief(target),
		})
	}

	// 按亲密度降序
	sort.Slice(items, func(i, j int) bool {
		return items[i]["affinity"].(int16) > items[j]["affinity"].(int16)
	})

	c.JSON(http.StatusOK, gin.H{
		"npc_id":    id,
		"name":      npc.Name,
		"count":     len(items),
		"counts":    byType,
		"relations": items,
	})
}

// GET /api/simverse/battle/recent?limit= : 近期战斗记录
func (s *Server) handleSimverseBattleRecent(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	recs := world.Battle().GetRecent(limit)
	items := make([]gin.H, 0, len(recs))
	for _, r := range recs {
		items = append(items, gin.H{
			"id":             r.ID,
			"tick":           r.Tick,
			"attacker_id":    r.AttackerID,
			"attacker_name":  r.AttackerName,
			"defender_id":    r.DefenderID,
			"defender_name":  r.DefenderName,
			"winner_id":      r.WinnerID,
			"loser_id":       r.LoserID,
			"outcome":        r.Outcome,
			"damage":         r.Damage,
			"attacker_hp":    r.AttackerHP,
			"defender_hp":    r.DefenderHP,
			"loot_gold":      r.LootGold,
			"log":            r.Log,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"total":   world.Battle().TotalBattles(),
		"count":   len(items),
		"battles": items,
	})
}

// GET /api/simverse/battle/rank?limit= : 胜场榜
func (s *Server) handleSimverseBattleRank(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	rank := world.Battle().GetRank(limit)
	items := make([]gin.H, 0, len(rank))
	for _, e := range rank {
		npc := world.GetNPC(e.NPCID, s.simverseMgr.rng)
		items = append(items, gin.H{
			"npc_id": e.NPCID,
			"name":   npc.Name,
			"wins":   e.Wins,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"count": len(items),
		"rank":  items,
	})
}

// POST /api/simverse/battle/simulate { attacker_id, defender_id } : 即时模拟一场战斗
func (s *Server) handleSimverseBattleSimulate(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	var body struct {
		AttackerID uint64 `json:"attacker_id"`
		DefenderID uint64 `json:"defender_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.AttackerID == 0 || body.DefenderID == 0 || body.AttackerID == body.DefenderID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid npc ids"})
		return
	}

	attacker := world.GetNPC(body.AttackerID, s.simverseMgr.rng)
	defender := world.GetNPC(body.DefenderID, s.simverseMgr.rng)
	rec := world.Battle().Simulate(attacker, defender, world.WorldTick(), s.simverseMgr.rng)

	c.JSON(http.StatusOK, gin.H{
		"id":             rec.ID,
		"tick":           rec.Tick,
		"attacker_id":    rec.AttackerID,
		"attacker_name":  rec.AttackerName,
		"defender_id":    rec.DefenderID,
		"defender_name":  rec.DefenderName,
		"winner_id":      rec.WinnerID,
		"loser_id":       rec.LoserID,
		"outcome":        rec.Outcome,
		"damage":         rec.Damage,
		"attacker_hp":    rec.AttackerHP,
		"defender_hp":    rec.DefenderHP,
		"loot_gold":      rec.LootGold,
		"log":            rec.Log,
	})
}

// GET /api/simverse/economy/prices?region=:id : 区域物价
func (s *Server) handleSimverseEconomyPrices(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	regionID := uint32(1)
	if v := c.Query("region"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			regionID = uint32(n)
		}
	}

	em := world.EconomyManager()
	c.JSON(http.StatusOK, em.GetRegionalStats(regionID))
}

// GET /api/simverse/economy/shocks : 价格冲击事件
func (s *Server) handleSimverseEconomyShocks(c *gin.Context) {
	if s.simverseMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simverse not initialized"})
		return
	}
	world := s.simverseMgr.World()
	if world == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "world not initialized"})
		return
	}

	em := world.EconomyManager()
	shocks := em.CheckPriceShocks()
	items := make([]gin.H, 0, len(shocks))
	for _, sh := range shocks {
		items = append(items, gin.H{
			"type":       sh.Type,
			"region_id":  sh.RegionID,
			"resource":   sh.Resource.String(),
			"price":      sh.Price,
			"change":     sh.Change,
			"message":    sh.Message,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"count": len(items),
		"items": items,
	})
}
