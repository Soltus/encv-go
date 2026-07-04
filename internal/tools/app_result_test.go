package tools

import (
	"errors"
	"strings"
	"testing"
)

// ─── IsSuccess / IsFailure ──────────────────────────────────

func TestAppResult_Success(t *testing.T) {
	r := NewAppResultSuccess(42)
	if !r.IsSuccess() {
		t.Error("expected IsSuccess=true")
	}
	if r.IsFailure() {
		t.Error("expected IsFailure=false")
	}
}

func TestAppResult_Failure(t *testing.T) {
	ae := NewAppError(AppErrNetworkTimeout, "x")
	r := NewAppResultFailure[int](ae)
	if r.IsSuccess() {
		t.Error("expected IsSuccess=false")
	}
	if !r.IsFailure() {
		t.Error("expected IsFailure=true")
	}
	if r.Error != ae {
		t.Error("Error should be the same instance")
	}
}

// ─── GetOrNull / GetOrElse ──────────────────────────────────

func TestAppResult_GetOrNull_Success(t *testing.T) {
	r := NewAppResultSuccess("hello")
	if got := r.GetOrNull(); got != "hello" {
		t.Errorf("GetOrNull = %q, want %q", got, "hello")
	}
}

func TestAppResult_GetOrNull_Failure(t *testing.T) {
	ae := NewAppError(AppErrUnknown, "x")
	r := NewAppResultFailure[string](ae)
	if got := r.GetOrNull(); got != "" {
		t.Errorf("GetOrNull on failure = %q, want empty string", got)
	}
}

func TestAppResult_GetOrElse(t *testing.T) {
	ae := NewAppError(AppErrUnknown, "x")
	r := NewAppResultFailure[int](ae)
	if got := r.GetOrElse(99); got != 99 {
		t.Errorf("GetOrElse = %d, want 99", got)
	}
}

// ─── Map ─────────────────────────────────────────────────────

func TestAppResult_Map_Success(t *testing.T) {
	r := NewAppResultSuccess(10)
	mapped := Map(r, func(n int) string { return string(rune('A' + n)) })
	if !mapped.IsSuccess() {
		t.Fatal("Map on success should be success")
	}
	if mapped.Data != "K" {
		t.Errorf("Map = %q, want %q", mapped.Data, "K")
	}
}

func TestAppResult_Map_Failure(t *testing.T) {
	ae := NewAppError(AppErrNetworkTimeout, "x")
	r := NewAppResultFailure[int](ae)
	mapped := Map(r, func(n int) string { return "" })
	if !mapped.IsFailure() {
		t.Fatal("Map on failure should remain failure")
	}
	if mapped.Error != ae {
		t.Error("Map on failure should preserve Error reference")
	}
}

// ─── OnSuccess / OnFailure ──────────────────────────────────

func TestAppResult_OnSuccess_RunsOnSuccess(t *testing.T) {
	var captured int
	NewAppResultSuccess(7).OnSuccess(func(v int) { captured = v })
	if captured != 7 {
		t.Errorf("OnSuccess handler not called, captured=%d", captured)
	}
}

func TestAppResult_OnSuccess_SkipsOnFailure(t *testing.T) {
	called := false
	ae := NewAppError(AppErrUnknown, "x")
	NewAppResultFailure[int](ae).OnSuccess(func(v int) { called = true })
	if called {
		t.Error("OnSuccess should not run on failure")
	}
}

func TestAppResult_OnFailure_RunsOnFailure(t *testing.T) {
	var captured *AppError
	ae := NewAppError(AppErrNetworkTimeout, "x")
	NewAppResultFailure[int](ae).OnFailure(func(e *AppError) { captured = e })
	if captured != ae {
		t.Error("OnFailure handler not called or wrong error")
	}
}

func TestAppResult_OnFailure_SkipsOnSuccess(t *testing.T) {
	called := false
	NewAppResultSuccess(1).OnFailure(func(e *AppError) { called = true })
	if called {
		t.Error("OnFailure should not run on success")
	}
}

// ─── RunCatching ─────────────────────────────────────────────

func TestRunCatching_Success(t *testing.T) {
	r := RunCatching(func() (string, error) {
		return "ok", nil
	})
	if !r.IsSuccess() {
		t.Fatal("RunCatching nil err should give success")
	}
	if r.Data != "ok" {
		t.Errorf("Data = %q, want %q", r.Data, "ok")
	}
}

func TestRunCatching_PreservesAppError(t *testing.T) {
	ae := NewAppError(AppErrRateLimited, "slow down")
	r := RunCatching(func() (int, error) {
		return 0, ae
	})
	if !r.IsFailure() {
		t.Fatal("expected failure")
	}
	if r.Error.Type != AppErrRateLimited {
		t.Errorf("Error type = %s, want %s", r.Error.Type, AppErrRateLimited)
	}
}

func TestRunCatching_UpgradesToolError(t *testing.T) {
	te := &ToolError{Code: CodeTimeout, Message: "deadline"}
	r := RunCatching(func() (int, error) {
		return 0, te
	})
	if !r.IsFailure() {
		t.Fatal("expected failure")
	}
	if r.Error.Type != AppErrNetworkTimeout {
		t.Errorf("ToolError{CodeTimeout} should upgrade to AppErrNetworkTimeout, got %s", r.Error.Type)
	}
}

func TestRunCatching_WrapsUnknownError(t *testing.T) {
	raw := errors.New("something weird")
	r := RunCatching(func() (int, error) {
		return 0, raw
	})
	if !r.IsFailure() {
		t.Fatal("expected failure")
	}
	if r.Error.Type != AppErrUnknown {
		t.Errorf("Unknown err should map to AppErrUnknown, got %s", r.Error.Type)
	}
	if !strings.Contains(r.Error.Message, "something weird") {
		t.Error("wrapped message should preserve original text")
	}
}

func TestRunCatching_FillsHumanMessage(t *testing.T) {
	r := RunCatching(func() (int, error) {
		return 0, errors.New("anything")
	})
	if r.Error.HumanMessage == "" {
		t.Error("RunCatching should auto-fill HumanMessage")
	}
}
