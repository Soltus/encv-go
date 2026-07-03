//go:build objectbox

package objectbox

//go:generate go run github.com/objectbox/objectbox-go/cmd/objectbox-gogen entities.go

// TaskEntity 任务实体（对应 tasksystem.TaskData）。
type TaskEntity struct {
	Id uint64 `objectbox:"id"`

	TaskID          string `objectbox:"unique index"`
	Type            string `objectbox:"index"`
	Status          string `objectbox:"index"`
	ServiceName     string `objectbox:"index"`
	MethodName      string
	TenantID        string `objectbox:"index"`
	TriggeredBy     string `objectbox:"index"`
	RunID           string `objectbox:"index"`

	SourcePath  string
	TargetPath  string
	OutputPath  string
	PluginName  string

	Progress int
	Phase    string

	Error         string
	ErrorDetail   string
	Warning       string
	WarningDetail string

	ContainerVersion int
	CipherMode       int
	CompressionMode  string
	ExtraFields      string
	Steps            string

	MountID            string
	MountSubPath       string
	TargetMountID      string
	TargetMountSubPath string
	Password           string
	SecondaryPassword  string

	DurationMs int64
	InputJSON  string
	OutputJSON string
	Attempts   int
	Priority   int
	TagsJSON   string

	CreatedAt   int64
	CompletedAt int64

	RollbackOf   string
	OriginalPath string
}

// SnapshotEntity 回滚快照实体。
type SnapshotEntity struct {
	Id           uint64 `objectbox:"id"`
	TaskID       string `objectbox:"index"`
	SnapshotType string
	Data         []byte
	CreatedAt    int64
}

// TrashEntity 回收站实体。
type TrashEntity struct {
	Id            uint64 `objectbox:"id"`
	TrashID       string `objectbox:"unique index"`
	OriginalPath  string
	TrashPath     string
	IsDirectory   bool
	Size          int64
	DeletedAt     int64
	TaskID        string `objectbox:"index"`
	RestoreTaskID string
	Metadata      string
}

// PerformanceMetricsEntity 性能指标实体。
type PerformanceMetricsEntity struct {
	Id               uint64 `objectbox:"id"`
	TaskID           string `objectbox:"unique index"`
	TaskType         string `objectbox:"index"`
	PluginName       string `objectbox:"index"`
	ContainerVersion int
	CipherMode       int
	CompressionMode  string
	SourceSize       int64
	OutputSize       int64
	SizeRatio        float64
	AvgThroughput    float64
	PeakThroughput   float64
	P50Throughput    float64
	P99Throughput    float64
	TotalDurationMs  int64
	PhaseTimingsJSON string
	Grade            string
	GradeScore       float64
	GradeReason      string
	CPUScore         float64
	CPULabel         string
	CreatedAt        int64
}

// CalibrationEntity 硬件校准实体（单例，Id=1）。
type CalibrationEntity struct {
	Id           uint64 `objectbox:"id"`
	CPUScore     float64
	AESThroughput float64
	CPULabel     string
	CalibratedAt int64
	GoVersion    string
	OS           string
	Arch         string
	NumCPU       int
}
