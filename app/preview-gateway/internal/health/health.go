package health

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"preview-gateway/internal/routing"
)

type Checker struct {
	client *http.Client
}

func New() *Checker {
	return &Checker{
		client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		},
	}
}

type upstreamResult struct {
	URL       string `json:"url"`
	Alive     bool   `json:"alive"`
	LatencyMs int64  `json:"latency_ms"`
	Required  bool   `json:"required"`
}

func (c *Checker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targets := routing.GetAllUpstreams()

	var wg sync.WaitGroup
	mu := sync.Mutex{}
	results := make(map[string]upstreamResult)

	for _, u := range targets {
		wg.Add(1)
		go func(u *routing.Upstream) {
			defer wg.Done()
			start := time.Now()
			resp, err := c.client.Get(u.Target)
			latency := time.Since(start).Milliseconds()
			alive := err == nil && resp != nil && resp.StatusCode < 500
			if resp != nil {
				resp.Body.Close()
			}
			mu.Lock()
			results[u.Name] = upstreamResult{
				URL:       u.Target,
				Alive:     alive,
				LatencyMs: latency,
				Required:  u.Required,
			}
			mu.Unlock()
		}(u)
	}

	wg.Wait()

	allRequiredOk := true
	for name, res := range results {
		for _, u := range targets {
			if u.Name == name && u.Required && !res.Alive {
				allRequiredOk = false
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !allRequiredOk {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        allRequiredOk,
		"upstreams": results,
		"version":   "go-v1",
	})
}
