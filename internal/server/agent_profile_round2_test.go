package server

// agent_profile_round2_test.go —— vNext Round 2 回归锁
//
// 锁定：运行 profile 通过 /api/runtime 对外声明，前端据此隐藏/禁用 mock 控制面
// （AGT-004）。production 必须声明 mock_allowed=false；test 才为 true。
// 未知/空 profile 一律按 production 处理（fail closed）。

import "testing"

func TestSnapshotRuntimeInfo_ExposesProfile(t *testing.T) {
	prod := &Server{runtimeProfile: ProfileProduction}
	info := prod.snapshotRuntimeInfo()
	if info.Profile != ProfileProduction {
		t.Fatalf("production profile = %q, want %q", info.Profile, ProfileProduction)
	}
	if info.MockAllowed {
		t.Fatal("production must declare mock_allowed=false")
	}

	testp := &Server{runtimeProfile: ProfileTest}
	tInfo := testp.snapshotRuntimeInfo()
	if tInfo.Profile != ProfileTest {
		t.Fatalf("test profile = %q, want %q", tInfo.Profile, ProfileTest)
	}
	if !tInfo.MockAllowed {
		t.Fatal("test profile must declare mock_allowed=true")
	}
}

func TestSnapshotRuntimeInfo_ZeroValueProfileIsProduction(t *testing.T) {
	// 零值 Server（未走 NewServer）也必须对外声明 production，不能"没声明=能演示"
	s := &Server{}
	info := s.snapshotRuntimeInfo()
	if info.Profile != ProfileProduction {
		t.Fatalf("zero-value profile = %q, want %q", info.Profile, ProfileProduction)
	}
	if info.MockAllowed {
		t.Fatal("zero-value server must not allow mock")
	}
}

func TestSnapshotRuntimeInfo_UnknownProfileFailsClosed(t *testing.T) {
	// 拼错 / 未知值不得意外开启 mock
	s := &Server{runtimeProfile: "TESTING"}
	info := s.snapshotRuntimeInfo()
	if info.MockAllowed {
		t.Fatalf("unknown profile %q must fail closed (mock_allowed=false)", s.runtimeProfile)
	}
}
