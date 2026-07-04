package service

// Phase 表示任务执行阶段的枚举常量。
// 字符串值必须与前端 app/encv-mobile/src/lib/workflow/types.ts 的 Phase 枚举一致。
type Phase string

const (
	PhaseCreated       Phase = "created"
	PhaseAnalyzing     Phase = "analyzing"
	PhaseInitializing  Phase = "initializing"
	PhasePreprocessing Phase = "preprocessing"
	PhaseEncrypting    Phase = "encrypting"
	PhaseDecrypting    Phase = "decrypting"
	PhasePacking       Phase = "packing"
	PhaseVerifying     Phase = "verifying"
	PhaseCompleted     Phase = "completed"
)
