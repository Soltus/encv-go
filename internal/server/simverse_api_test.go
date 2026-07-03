package server

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Soltus/encv-go/internal/simverse"
	"github.com/gin-gonic/gin"
)

func setupSimverseTestServer() (*gin.Engine, *SimverseManager) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	mgr := NewSimverseManager()
	s := &Server{simverseMgr: mgr}

	simGroup := r.Group("/api/simverse")
	{
		simGroup.GET("/world/state", s.handleSimverseWorldState)
		simGroup.GET("/world/config", s.handleSimverseWorldConfig)
		simGroup.POST("/world/config", s.handleSimverseSetConfig)
		simGroup.POST("/world/control", s.handleSimverseWorldControl)
		simGroup.GET("/npc/list", s.handleSimverseNPCList)
		simGroup.GET("/npc/:id", s.handleSimverseNPCDetail)
		simGroup.GET("/focus", s.handleSimverseFocusList)
		simGroup.POST("/focus", s.handleSimverseSetFocus)
		simGroup.GET("/perf/metrics", s.handleSimversePerfMetrics)
	}

	return r, mgr
}

func TestSimverseAPI_WorldState(t *testing.T) {
	r, mgr := setupSimverseTestServer()
	defer mgr.Stop()

	req := httptest.NewRequest("GET", "/api/simverse/world/state", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if resp["running"] == nil {
		t.Error("Missing 'running' field")
	}
	if resp["tick"] == nil {
		t.Error("Missing 'tick' field")
	}
	if resp["npc_count"] == nil {
		t.Error("Missing 'npc_count' field")
	}

	t.Logf("World state: tick=%v npc_count=%v total_mb=%.2f",
		resp["tick"], resp["npc_count"], resp["total_mb"])
}

func TestSimverseAPI_WorldConfig(t *testing.T) {
	r, mgr := setupSimverseTestServer()
	defer mgr.Stop()

	req := httptest.NewRequest("GET", "/api/simverse/world/config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if resp["tier"] == nil {
		t.Error("Missing 'tier' field")
	}
	if resp["tier_name"] == nil {
		t.Error("Missing 'tier_name' field")
	}
	if resp["event_rate_mul"] == nil {
		t.Error("Missing 'event_rate_mul' field")
	}

	t.Logf("Config: tier=%v name=%v rate=%.1fx",
		resp["tier"], resp["tier_name"], resp["event_rate_mul"])
}

func TestSimverseAPI_SetConfig(t *testing.T) {
	r, mgr := setupSimverseTestServer()
	defer mgr.Stop()

	body := bytes.NewBufferString(`{"tier": "foreground"}`)
	req := httptest.NewRequest("POST", "/api/simverse/world/config", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if resp["tier_name"] != "foreground" {
		t.Errorf("Expected tier_name=foreground, got %v", resp["tier_name"])
	}

	t.Logf("Set config OK: tier=%v rate=%.1fx",
		resp["tier"], resp["event_rate_mul"])

	body2 := bytes.NewBufferString(`{"tier": "fg_idle"}`)
	req2 := httptest.NewRequest("POST", "/api/simverse/world/config", body2)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("Expected 200 for fg_idle, got %d", w2.Code)
	}
	t.Log("fg_idle tier set OK")

	body3 := bytes.NewBufferString(`{"tier": "invalid"}`)
	req3 := httptest.NewRequest("POST", "/api/simverse/world/config", body3)
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)

	if w3.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid tier, got %d", w3.Code)
	}
	t.Log("Invalid tier correctly rejected")
}

func TestSimverseAPI_WorldControl(t *testing.T) {
	r, mgr := setupSimverseTestServer()
	defer mgr.Stop()

	body := bytes.NewBufferString(`{"action": "step"}`)
	req := httptest.NewRequest("POST", "/api/simverse/world/control", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for step, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	t.Logf("Step OK: status=%v tick=%v", resp["status"], resp["tick"])

	body2 := bytes.NewBufferString(`{"action": "start"}`)
	req2 := httptest.NewRequest("POST", "/api/simverse/world/control", body2)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("Expected 200 for start, got %d", w2.Code)
	}
	t.Log("Start OK")

	mgr.Stop()
	t.Log("Stop OK (via manager)")
}

func TestSimverseAPI_NPCList(t *testing.T) {
	r, mgr := setupSimverseTestServer()
	defer mgr.Stop()

	req := httptest.NewRequest("GET", "/api/simverse/npc/list?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if resp["page"] != float64(1) {
		t.Errorf("Expected page=1, got %v", resp["page"])
	}
	if resp["page_size"] != float64(10) {
		t.Errorf("Expected page_size=10, got %v", resp["page_size"])
	}

	items, ok := resp["items"].([]interface{})
	if !ok {
		t.Fatal("Missing items array")
	}
	if len(items) != 10 {
		t.Errorf("Expected 10 items, got %d", len(items))
	}

	firstNPC := items[0].(map[string]interface{})
	if firstNPC["id"] == nil {
		t.Error("First NPC missing 'id'")
	}
	if firstNPC["name"] == nil {
		t.Error("First NPC missing 'name'")
	}
	if firstNPC["species"] == nil {
		t.Error("First NPC missing 'species'")
	}

	t.Logf("NPC list: page=%v total=%v first=%v(%v)",
		resp["page"], resp["total"], firstNPC["name"], firstNPC["species"])
}

func TestSimverseAPI_NPCDetail(t *testing.T) {
	r, mgr := setupSimverseTestServer()
	defer mgr.Stop()

	req := httptest.NewRequest("GET", "/api/simverse/npc/42", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if resp["id"] != float64(42) {
		t.Errorf("Expected id=42, got %v", resp["id"])
	}
	if resp["name"] == nil {
		t.Error("Missing 'name' field")
	}
	if resp["skills"] == nil {
		t.Error("Missing 'skills' field")
	}
	if resp["inventory"] == nil {
		t.Error("Missing 'inventory' field")
	}
	if resp["life_stage"] == nil {
		t.Error("Missing 'life_stage' field")
	}

	skills, _ := resp["skills"].(map[string]interface{})
	t.Logf("NPC #42: name=%v stage=%v skills=%d",
		resp["name"], resp["life_stage"], len(skills))
}

func TestSimverseAPI_FocusList(t *testing.T) {
	r, mgr := setupSimverseTestServer()
	defer mgr.Stop()

	req := httptest.NewRequest("GET", "/api/simverse/focus", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	count := int(resp["count"].(float64))
	t.Logf("Focus NPCs: %d", count)

	if count != 50 {
		t.Errorf("Expected 50 focus NPCs, got %d", count)
	}
}

func TestSimverseAPI_SetFocus(t *testing.T) {
	r, mgr := setupSimverseTestServer()
	defer mgr.Stop()

	body := bytes.NewBufferString(`{"npcs": [{"id": 9999, "level": "core"}, {"id": 8888, "level": "player"}]}`)
	req := httptest.NewRequest("POST", "/api/simverse/focus", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	count := int(resp["count"].(float64))
	t.Logf("After set focus: %d NPCs", count)

	world := mgr.World()
	lvl := world.FocusLevel(9999)
	if lvl != simverse.FocusCore {
		t.Errorf("Expected focus level core for 9999, got %v", lvl)
	}
	lvl2 := world.FocusLevel(8888)
	if lvl2 != simverse.FocusPlayer {
		t.Errorf("Expected focus level player for 8888, got %v", lvl2)
	}
	t.Log("✅ Focus levels set correctly")
}

func TestSimverseAPI_PerfMetrics(t *testing.T) {
	r, mgr := setupSimverseTestServer()
	defer mgr.Stop()

	for i := 0; i < 100; i++ {
		world := mgr.World()
		world.Tick(rand.New(rand.NewSource(42)))
	}

	req := httptest.NewRequest("GET", "/api/simverse/perf/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	t.Logf("Perf metrics: avg_tick_ns=%v tps=%.0f samples=%v total_mb=%.2f",
		resp["avg_tick_ns"], resp["ticks_per_sec"], resp["samples"], resp["total_mb"])

	if resp["avg_tick_ns"] == nil {
		t.Error("Missing avg_tick_ns")
	}
	if resp["ticks_per_sec"] == nil {
		t.Error("Missing ticks_per_sec")
	}
}

func TestSimverseAPI_AllEndpoints(t *testing.T) {
	r, mgr := setupSimverseTestServer()
	defer mgr.Stop()

	endpoints := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{"GET", "/api/simverse/world/state", "", 200},
		{"GET", "/api/simverse/world/config", "", 200},
		{"POST", "/api/simverse/world/config", `{"tier":"background"}`, 200},
		{"POST", "/api/simverse/world/control", `{"action":"step"}`, 200},
		{"GET", "/api/simverse/npc/list", "", 200},
		{"GET", "/api/simverse/npc/0", "", 200},
		{"GET", "/api/simverse/npc/9999", "", 200},
		{"GET", "/api/simverse/focus", "", 200},
		{"POST", "/api/simverse/focus", `{"npcs":[{"id":1234,"level":"near"}]}`, 200},
		{"GET", "/api/simverse/perf/metrics", "", 200},
		{"POST", "/api/simverse/world/config", `{"tier":"invalid"}`, 400},
		{"POST", "/api/simverse/world/control", `{"action":"invalid"}`, 400},
	}

	passed := 0
	failed := 0

	for _, ep := range endpoints {
		var body *bytes.Buffer
		if ep.body != "" {
			body = bytes.NewBufferString(ep.body)
		} else {
			body = bytes.NewBuffer(nil)
		}

		req := httptest.NewRequest(ep.method, ep.path, body)
		if ep.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code == ep.status {
			passed++
			t.Logf("✅ %s %s -> %d", ep.method, ep.path, w.Code)
		} else {
			failed++
			t.Errorf("❌ %s %s: expected %d, got %d: %s",
				ep.method, ep.path, ep.status, w.Code, w.Body.String())
		}
	}

	t.Logf("=== Summary: %d passed, %d failed ===", passed, failed)
}
