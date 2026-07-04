// Command agent-demo is the stand-alone HTTP service for the
// in-process Go agent. See spec at
// `.trae/specs/go-in-process-agent/spec.md` for the full
// design.
//
// Build & run:
//
//	go build -o bin/agent-demo ./agent/cmd/agent-demo
//	./bin/agent-demo
//
// Configuration is loaded from `agent_settings` inside
// `~/.encv/config.user.json` (or
// `$CONFIG_USER_JSON` / `/workspace/config.user.json` as
// fallback). See config_loader.go for the full resolution
// order.
package main
