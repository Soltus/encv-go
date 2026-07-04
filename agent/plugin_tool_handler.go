package agent

import "time"

// nowMillis is the agent's wall-clock accessor, factored out so
// tests can monkey-patch it if needed.
func nowMillis() int64 {
	return time.Now().UnixMilli()
}
