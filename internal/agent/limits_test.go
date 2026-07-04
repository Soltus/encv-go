package agent

import (
	"errors"
	"testing"
)

func TestRecursionGuard_NewDefaults(t *testing.T) {
	g := NewRecursionGuard(0, 0) // max=0 应该用默认值
	if g.Max() != MaxToolIterations {
		t.Errorf("default max should be %d, got %d", MaxToolIterations, g.Max())
	}
	if g.Current() != 0 {
		t.Errorf("current should be 0, got %d", g.Current())
	}
}

func TestRecursionGuard_NewCustomMax(t *testing.T) {
	g := NewRecursionGuard(5, 10)
	if g.Max() != 10 {
		t.Errorf("custom max should be 10, got %d", g.Max())
	}
	if g.Current() != 5 {
		t.Errorf("current should be 5, got %d", g.Current())
	}
}

func TestRecursionGuard_Increment_UnderLimit(t *testing.T) {
	g := NewRecursionGuard(0, 20)
	for i := 0; i < 19; i++ {
		if err := g.Increment(); err != nil {
			t.Errorf("increment at i=%d should not error, got: %v", i, err)
		}
	}
}

func TestRecursionGuard_Increment_AtLimit(t *testing.T) {
	g := NewRecursionGuard(0, 5)
	for i := 0; i < 5; i++ {
		_ = g.Increment()
	}
	// 第 6 次应该超限
	err := g.Increment()
	if !errors.Is(err, ErrMaxToolIterationsExceeded) {
		t.Errorf("6th increment should return ErrMaxToolIterationsExceeded, got: %v", err)
	}
}

func TestRecursionGuard_Increment_NegativeCurrent(t *testing.T) {
	g := NewRecursionGuard(-3, 5)
	if g.Current() != 0 {
		t.Errorf("negative current should be normalized to 0, got %d", g.Current())
	}
}

func TestRecursionGuard_Remaining(t *testing.T) {
	g := NewRecursionGuard(0, 20)
	if g.Remaining() != 20 {
		t.Errorf("remaining should be 20, got %d", g.Remaining())
	}
	_ = g.Increment()
	_ = g.Increment()
	if g.Remaining() != 18 {
		t.Errorf("remaining should be 18, got %d", g.Remaining())
	}
}

func TestRecursionGuard_Remaining_ClampsAtZero(t *testing.T) {
	g := NewRecursionGuard(25, 5)
	if g.Remaining() != 0 {
		t.Errorf("remaining should clamp to 0, got %d", g.Remaining())
	}
}

func TestRecursionGuard_Reset(t *testing.T) {
	g := NewRecursionGuard(0, 5)
	for i := 0; i < 3; i++ {
		_ = g.Increment()
	}
	g.Reset()
	if g.Current() != 0 {
		t.Errorf("after reset current should be 0, got %d", g.Current())
	}
	if g.Remaining() != 5 {
		t.Errorf("after reset remaining should be 5, got %d", g.Remaining())
	}
}
