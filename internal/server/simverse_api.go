package server

import (
	"log/slog"
	"math/rand"
	"net/http"
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
		npcs = append(npcs, gin.H{
			"id":          npc.ID,
			"name":        npc.Name,
			"species":     npc.Species.String(),
			"gender":      npc.Gender.String(),
			"age":         npc.Age,
			"life_stage":  npc.LifeStage.String(),
			"profession":  npc.Profession.String(),
			"level":       npc.Level,
			"health":      npc.Health,
			"max_health":  npc.MaxHealth,
			"energy":      npc.Energy,
			"max_energy":  npc.MaxEnergy,
			"is_alive":    npc.IsAlive,
			"wealth_tier": npc.WealthTier,
			"social_tier": npc.SocialTier,
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
