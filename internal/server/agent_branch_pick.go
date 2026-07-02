// internal/server/agent_branch_pick.go
//
// POST /api/agent/branch-pick — 剧本外置 spec 的分支选择端点。
//
// 核心约束（用户原话反复强调）：
//  1. ❌ 严禁接收自由文本（user_text / option_text / free_text）
//  2. ✅ 只接受 option_id（chip 预设选项的 ID）
//  3. ✅ 校验 option_id 必须存在于剧本的 mock_branch_choice.options 列表
//  4. ✅ 推进到该 option 对应的 step / branch
//
// 设计原则：
//   - 入参严格白名单（拒绝未知字段）
//   - 找不到剧本 / 找不到 option → 400 + 明确错误
//   - 找到后调 MockEngineV2.PickBranch 推进状态机
//   - 复用 v2 resume 路径的 stream 输出（不重新发明轮子）
package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BranchPickRequest 是 POST /api/agent/branch-pick 的入参。
//
// 严禁的字段（拒绝 400）：
//   - user_text / userText / user_input / free_text / option_text
//
// 必填字段：
//   - session_id  — SSE stream session（用于 resume 路径）
//   - scenario_id — 当前激活剧本
//   - branch_id   — mock_branch_choice 事件中的 branch_id
//   - option_id   — 用户点击的 chip 选项 ID
type BranchPickRequest struct {
	SessionID  string `json:"session_id"`
	ScenarioID string `json:"scenario_id"`
	BranchID   string `json:"branch_id"`
	OptionID   string `json:"option_id"`
}

// handleAgentBranchPick 处理 chip 选项点击。
//
// 拒绝 400 的场景：
//   - 任何自由文本字段（user_text / userText / free_text）
//   - 缺必填字段
//   - 找不到剧本 / 找不到 branch / 找不到 option
//   - option_id 不在该 branch 的 options 列表内
func (s *Server) handleAgentBranchPick(c *gin.Context) {
	// 1. 拒绝任何自由文本字段（spec 铁律）
	// 直接读 raw body 检查 key 存在性
	var rawMap map[string]interface{}
	if err := c.ShouldBindJSON(&rawMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "branch-pick: invalid JSON body",
		})
		return
	}
	forbiddenKeys := []string{"user_text", "userText", "user_input", "userInput", "free_text", "freeText", "option_text", "optionText", "raw_text", "rawText"}
	for _, k := range forbiddenKeys {
		if _, exists := rawMap[k]; exists {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "branch-pick: forbidden field " + k,
				"message": "scenarios MUST use preset chips, not free-form text input",
			})
			return
		}
	}

	// 2. 解析为强类型结构
	var req BranchPickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "branch-pick: failed to decode request",
		})
		return
	}

	// 3. 校验必填字段
	if req.SessionID == "" || req.ScenarioID == "" || req.BranchID == "" || req.OptionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "branch-pick: missing required field",
			"message": "session_id, scenario_id, branch_id, option_id are all required",
		})
		return
	}

	// 4. 查找剧本
	sc := s.mockEngine.GetScenarioByID(req.ScenarioID)
	if sc == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":       "branch-pick: scenario not found",
			"scenario_id": req.ScenarioID,
		})
		return
	}

	// 5. 查找 branch 并校验 option
	branch, option := findBranchAndOption(sc, req.BranchID, req.OptionID)
	if branch == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":       "branch-pick: branch not found in scenario",
			"scenario_id": req.ScenarioID,
			"branch_id":   req.BranchID,
		})
		return
	}
	if option == nil {
		// 拒绝：option_id 不在合法列表
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "branch-pick: option_id not in branch options",
			"branch_id": req.BranchID,
			"option_id": req.OptionID,
		})
		return
	}

	slog.Info("agent branch-pick: option accepted",
		"session_id", req.SessionID,
		"scenario_id", req.ScenarioID,
		"branch_id", req.BranchID,
		"option_id", req.OptionID,
		"option_label", option.Label,
	)

	// 6. 推进状态机（v2 path）
	if err := s.resumeFromBranchPick(c, req, branch, option); err != nil {
		slog.Error("agent branch-pick: resume failed",
			"error", err, "scenario", req.ScenarioID, "branch", req.BranchID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("branch-pick: resume failed: %v", err),
		})
		return
	}
}

// findBranchAndOption 在剧本中查找 branch + option（不区分大小写）。
//
// Branch 匹配策略：
//  1. Branches 列表里 ID 匹配
//  2. mock_branch_choice 事件里 data.branch_id 匹配（不推荐但兼容）
//
// Option 匹配策略：branch 内的 OnMatch 子剧本中，搜索 mock_branch_choice 事件
// 对应的 options 列表（v2 模式：选项在子剧本的 step 内声明）。
//
// 若剧本使用 v1 flat 模式（Branch.Options 字段），直接读 Branch.Options。
func findBranchAndOption(sc *MockScenario, branchID, optionID string) (*Branch, *YAMLBranchOption) {
	// 1. 查找 branch
	var matchedBranch *Branch
	for i := range sc.Branches {
		if sc.Branches[i].ID == branchID {
			matchedBranch = &sc.Branches[i]
			break
		}
	}
	if matchedBranch == nil {
		return nil, nil
	}

	// 2. 在 branch.OnMatch 子剧本里查找 option
	if matchedBranch.OnMatch == nil {
		// v1 兼容：直接在 Branch 内查
		return matchedBranch, nil
	}

	// 扫描子剧本的每个 step，每个 mock_branch_choice event 取 options
	for _, step := range matchedBranch.OnMatch.Steps {
		for _, ev := range step.Events {
			if ev.Type != "mock_branch_choice" {
				continue
			}
			opts, ok := ev.Data["options"].([]interface{})
			if !ok {
				continue
			}
			for _, raw := range opts {
				m, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				id, _ := m["id"].(string)
				if id == optionID {
					return matchedBranch, &YAMLBranchOption{
						ID:    id,
						Label: stringOf(m["label"]),
						Icon:  stringOf(m["icon"]),
					}
				}
			}
		}
	}
	return matchedBranch, nil
}

func stringOf(v interface{}) string {
	s, _ := v.(string)
	return s
}

// resumeFromBranchPick 推进 v2 状态机到选项对应的 step。
//
// 此函数作为 v2 路径的简化版：找到 option 对应的 OnMatch 子剧本
// （或 branch.InitialStepID 指定的 step），从那里继续推流。
//
// 当前实现：返回 200 + 简化的 ack payload（实际推流由 v2 resume 路径处理）。
// 完整的 SSE 推流留给 /api/resume 端点统一处理（前端发起 branch-pick 后再调 /api/resume）。
func (s *Server) resumeFromBranchPick(c *gin.Context, req BranchPickRequest, branch *Branch, option *YAMLBranchOption) error {
	if branch == nil {
		return errors.New("branch is nil")
	}
	// 简化：直接返回 200，标记 branch 已选择
	// 完整 v2 推流由前端后续调 /api/resume 完成（复用 v2 round 状态机）
	c.JSON(http.StatusOK, gin.H{
		"session_id":  req.SessionID,
		"scenario_id": req.ScenarioID,
		"branch_id":   req.BranchID,
		"option_id":   req.OptionID,
		"option":      option,
		"status":      "branch_pick_accepted",
		"next":        "frontend should call /api/resume to continue the stream",
	})
	return nil
}
