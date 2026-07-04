package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

type TaskWithWarnings struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	SourcePath       string          `json:"sourcePath"`
	Status           string          `json:"status"`
	Progress         int             `json:"progress"`
	Error            string          `json:"error,omitempty"`
	Warning          string          `json:"warning,omitempty"`
	WarningDetail    []WarningDetail `json:"warningDetail,omitempty"`
	ContainerVersion int             `json:"containerVersion,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	CompletedAt      *time.Time      `json:"completedAt,omitempty"`
}

type WarningDetail struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

func TestTask_WarningFields_Serialization(t *testing.T) {
	now := time.Now()
	details := []WarningDetail{
		{Code: "SIZE_MISMATCH", Message: "Original and decrypted file sizes differ", Severity: "warning"},
		{Code: "STRUCT_CHECK_SKIPPED", Message: "MP4 structure check skipped (re-encoded output)", Severity: "info"},
	}

	task := TaskWithWarnings{
		ID:            "task-001",
		Type:          "encrypt",
		SourcePath:    "/input/video.mp4",
		Status:        "completed",
		Progress:      100,
		Warning:       "2 warnings during verification",
		WarningDetail: details,
		CreatedAt:     now,
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("failed to marshal task with warnings: %v", err)
	}

	dataStr := string(data)

	if !strings.Contains(dataStr, `"warning"`) {
		t.Error("marshaled JSON should contain 'warning' field")
	}
	if !strings.Contains(dataStr, `"warningDetail"`) {
		t.Error("marshaled JSON should contain 'warningDetail' field")
	}
	if !strings.Contains(dataStr, "2 warnings during verification") {
		t.Error("marshaled JSON should contain warning message text")
	}
	if !strings.Contains(dataStr, "SIZE_MISMATCH") {
		t.Error("marshaled JSON should contain warning detail code")
	}
	if !strings.Contains(dataStr, "STRUCT_CHECK_SKIPPED") {
		t.Error("marshaled JSON should contain second warning detail code")
	}

	var decoded TaskWithWarnings
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal task with warnings: %v", err)
	}

	if decoded.Warning != task.Warning {
		t.Errorf("Warning mismatch after roundtrip: got %q, want %q", decoded.Warning, task.Warning)
	}
	if len(decoded.WarningDetail) != len(task.WarningDetail) {
		t.Errorf("WarningDetail count mismatch: got %d, want %d", len(decoded.WarningDetail), len(task.WarningDetail))
	}
	for i, detail := range decoded.WarningDetail {
		if detail.Code != task.WarningDetail[i].Code {
			t.Errorf("WarningDetail[%d] Code mismatch: got %q, want %q", i, detail.Code, task.WarningDetail[i].Code)
		}
		if detail.Message != task.WarningDetail[i].Message {
			t.Errorf("WarningDetail[%d] Message mismatch: got %q, want %q", i, detail.Message, task.WarningDetail[i].Message)
		}
		if detail.Severity != task.WarningDetail[i].Severity {
			t.Errorf("WarningDetail[%d] Severity mismatch: got %q, want %q", i, detail.Severity, task.WarningDetail[i].Severity)
		}
	}
}

func TestTask_WarningFields_Optional(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		task          TaskWithWarnings
		expectWarning bool
		expectDetail  bool
	}{
		{
			name: "no warnings - clean completion",
			task: TaskWithWarnings{
				ID:         "task-clean",
				Type:       "encrypt",
				SourcePath: "/input/video.mp4",
				Status:     "completed",
				Progress:   100,
				CreatedAt:  now,
			},
			expectWarning: false,
			expectDetail:  false,
		},
		{
			name: "with warning only",
			task: TaskWithWarnings{
				ID:         "task-warn-only",
				Type:       "encrypt",
				SourcePath: "/input/video.mp4",
				Status:     "completed",
				Progress:   100,
				Warning:    "minor issue detected",
				CreatedAt:  now,
			},
			expectWarning: true,
			expectDetail:  false,
		},
		{
			name: "with warning detail only",
			task: TaskWithWarnings{
				ID:         "task-detail-only",
				Type:       "encrypt",
				SourcePath: "/input/video.mp4",
				Status:     "completed",
				Progress:   100,
				WarningDetail: []WarningDetail{
					{Code: "TEST_CODE", Message: "test message", Severity: "info"},
				},
				CreatedAt: now,
			},
			expectWarning: false,
			expectDetail:  true,
		},
		{
			name: "empty warning string should be omitted",
			task: TaskWithWarnings{
				ID:         "task-empty-warn",
				Type:       "encrypt",
				SourcePath: "/input/video.mp4",
				Status:     "completed",
				Progress:   100,
				Warning:    "",
				CreatedAt:  now,
			},
			expectWarning: false,
			expectDetail:  false,
		},
		{
			name: "empty warning detail slice should be omitted",
			task: TaskWithWarnings{
				ID:            "task-empty-detail",
				Type:          "encrypt",
				SourcePath:    "/input/video.mp4",
				Status:        "completed",
				Progress:      100,
				WarningDetail: []WarningDetail{},
				CreatedAt:     now,
			},
			expectWarning: false,
			expectDetail:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.task)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			dataStr := string(data)

			hasWarning := strings.Contains(dataStr, `"warning"`)
			hasDetail := strings.Contains(dataStr, `"warningDetail"`)

			if hasWarning != tt.expectWarning {
				if tt.expectWarning {
					t.Errorf("expected 'warning' field in JSON but it was omitted")
				} else {
					t.Errorf("expected 'warning' field to be omitted but it was present")
				}
			}
			if hasDetail != tt.expectDetail {
				if tt.expectDetail {
					t.Errorf("expected 'warningDetail' field in JSON but it was omitted")
				} else {
					t.Errorf("expected 'warningDetail' field to be omitted but it was present")
				}
			}
		})
	}
}

func TestTask_CompletedWithWarning_DisplayLogic(t *testing.T) {
	tests := []struct {
		name               string
		status             string
		warning            string
		detailCount        int
		isValidCombination bool
	}{
		{
			name:               "completed with single warning is valid",
			status:             "completed",
			warning:            "1 warning during encryption",
			detailCount:        1,
			isValidCombination: true,
		},
		{
			name:               "completed with multiple warnings is valid",
			status:             "completed",
			warning:            "3 warnings during verification",
			detailCount:        3,
			isValidCombination: true,
		},
		{
			name:               "completed without warning is valid",
			status:             "completed",
			warning:            "",
			detailCount:        0,
			isValidCombination: true,
		},
		{
			name:               "running status with warning is valid (early detection)",
			status:             "running",
			warning:            "potential issue detected at 50%",
			detailCount:        1,
			isValidCombination: true,
		},
		{
			name:               "failed status with warning is valid (contextual info)",
			status:             "failed",
			warning:            "failure preceded by 2 warnings",
			detailCount:        2,
			isValidCombination: true,
		},
		{
			name:               "queued status should not have warnings",
			status:             "queued",
			warning:            "",
			detailCount:        0,
			isValidCombination: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			details := make([]WarningDetail, tt.detailCount)
			for i := range details {
				details[i] = WarningDetail{
					Code:     fmt.Sprintf("WARN_%03d", i+1),
					Message:  fmt.Sprintf("warning detail #%d", i+1),
					Severity: "info",
				}
			}

			task := TaskWithWarnings{
				ID:            fmt.Sprintf("task-%s", strings.ToLower(tt.status)),
				Type:          "encrypt",
				SourcePath:    "/input/test.mp4",
				Status:        tt.status,
				Progress:      100,
				Warning:       tt.warning,
				WarningDetail: details,
				CreatedAt:     now,
			}

			data, err := json.Marshal(task)
			if err != nil {
				t.Fatalf("failed to marshal task: %v", err)
			}

			var decoded TaskWithWarnings
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal task: %v", err)
			}

			if decoded.Status != tt.status {
				t.Errorf("Status mismatch: got %q, want %q", decoded.Status, tt.status)
			}
			if decoded.Warning != tt.warning {
				t.Errorf("Warning mismatch: got %q, want %q", decoded.Warning, tt.warning)
			}
			if len(decoded.WarningDetail) != tt.detailCount {
				t.Errorf("WarningDetail count mismatch: got %d, want %d", len(decoded.WarningDetail), tt.detailCount)
			}

			if tt.warning != "" && tt.detailCount > 0 {
				expectedPrefix := fmt.Sprintf("%d warning", tt.detailCount)
				if !strings.HasPrefix(decoded.Warning, expectedPrefix) &&
					!strings.Contains(decoded.Warning, fmt.Sprintf("%d", tt.detailCount)) {
					t.Logf("Note: warning text '%s' does not explicitly mention count %d (may be intentional)", decoded.Warning, tt.detailCount)
				}
			}
		})
	}
}
