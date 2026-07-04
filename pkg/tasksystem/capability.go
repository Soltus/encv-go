package tasksystem

import "sync"

// CapabilityTest 引擎专属能力测试。
//
// 每个引擎可以注册任意多个专属能力测试，
// 数据库自动化测试 runner 会自动发现并运行这些测试。
//
// 这是引擎差异化价值的核心体现——
// 如果所有引擎都只跑 CRUD 交集，那集成多引擎毫无意义。
type CapabilityTest struct {
	ID          string
	Name        string
	Description string
	Category    string // "performance" | "feature" | "consistency"
	Engine      string // 对应 Store.EngineName()
	Run         func(store Store) (map[string]any, error)
}

var (
	capabilityRegistryMu sync.RWMutex
	capabilityRegistry   = map[string][]CapabilityTest{}
)

// RegisterCapabilityTest 注册引擎专属能力测试。
// 通常在引擎包的 init() 中调用。
func RegisterCapabilityTest(test CapabilityTest) {
	capabilityRegistryMu.Lock()
	defer capabilityRegistryMu.Unlock()
	capabilityRegistry[test.Engine] = append(
		capabilityRegistry[test.Engine], test,
	)
}

// GetCapabilityTests 获取指定引擎的所有专属能力测试。
func GetCapabilityTests(engineName string) []CapabilityTest {
	capabilityRegistryMu.RLock()
	defer capabilityRegistryMu.RUnlock()
	tests := make([]CapabilityTest, len(capabilityRegistry[engineName]))
	copy(tests, capabilityRegistry[engineName])
	return tests
}

// AllCapabilityTests 返回所有注册的能力测试，按引擎分组。
func AllCapabilityTests() map[string][]CapabilityTest {
	capabilityRegistryMu.RLock()
	defer capabilityRegistryMu.RUnlock()
	result := make(map[string][]CapabilityTest, len(capabilityRegistry))
	for k, v := range capabilityRegistry {
		dst := make([]CapabilityTest, len(v))
		copy(dst, v)
		result[k] = dst
	}
	return result
}
