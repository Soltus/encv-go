package server

// agent_stats.go — 拆分自 agent_api.go

func maxEventID(events []AgentEvent) int64 {
	if len(events) == 0 {
		return 0
	}
	max := events[0].ID
	for _, e := range events[1:] {
		if e.ID > max {
			max = e.ID
		}
	}
	return max
}
