package simverse

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type WorldSaveState int

const (
	SaveStateNormal   WorldSaveState = 0
	SaveStateLowSpace WorldSaveState = 1
	SaveStatePaused   WorldSaveState = 2
)

type WorldMetadata struct {
	WorldID       uint64    `json:"world_id"`
	WorldName     string    `json:"world_name"`
	CreatedAt     time.Time `json:"created_at"`
	LastSavedAt   time.Time `json:"last_saved_at"`
	Tick          uint32    `json:"tick"`
	CurrentEra    uint16    `json:"current_era"`
	PerfTier      string    `json:"perf_tier"`
	NPCCount      uint32    `json:"npc_count"`
	FocusCount    uint32    `json:"focus_count"`
	SaveVersion   int       `json:"save_version"`
	SaveState     string    `json:"save_state"`
	DataDir       string    `json:"-"`
}

const currentSaveVersion = 1

type WorldPersistence struct {
	mu       sync.RWMutex
	dataDir  string
	meta     WorldMetadata
	metaPath string

	globalPlaceholderPath string
	placeholderSize       int64
	placeholderCreated    bool
}

const (
	StorageLevelNormal   = "normal"
	StorageLevelLow      = "low"
	StorageLevelCritical = "critical"
)

const (
	StorageReserveRatio     = 0.01                // 1% 相对值（基于磁盘总容量）
	StorageReserveMinBytes  = 1 * 1024 * 1024 * 1024  // 1GB 绝对值硬下限
	PlaceholderMaxBytes     = 10 * 1024 * 1024 * 1024 // 占位文件最大 10GB
	PlaceholderMinBytes     = 10 * 1024 * 1024        // 占位文件最小 10MB
	StorageLowBytes       = 200 * 1024 * 1024  // 200MB = YELLOW
	StorageCriticalBytes  = 50 * 1024 * 1024   // 50MB = RED
)

func calcPlaceholderSize(totalBytes int64, availableBytes int64) (int64, bool) {
	reserveBytes := int64(float64(totalBytes) * StorageReserveRatio)
	if reserveBytes < StorageReserveMinBytes {
		reserveBytes = StorageReserveMinBytes
	}

	maxPlaceholder := availableBytes - reserveBytes
	if maxPlaceholder < PlaceholderMinBytes {
		return 0, false
	}
	if maxPlaceholder > PlaceholderMaxBytes {
		maxPlaceholder = PlaceholderMaxBytes
	}

	return maxPlaceholder, true
}

func NewWorldPersistence(dataDir string, worldName string) *WorldPersistence {
	wp := &WorldPersistence{
		dataDir:               dataDir,
		metaPath:              filepath.Join(dataDir, "world_meta.json"),
		globalPlaceholderPath: filepath.Join(dataDir, ".storage_reserve.tmp"),
	}

	if data, err := os.ReadFile(wp.metaPath); err == nil {
		var meta WorldMetadata
		if json.Unmarshal(data, &meta) == nil {
			wp.meta = meta
			wp.meta.DataDir = dataDir
			return wp
		}
	}

	wp.meta = WorldMetadata{
		WorldID:     uint64(time.Now().UnixNano()),
		WorldName:   worldName,
		CreatedAt:   time.Now(),
		LastSavedAt: time.Now(),
		Tick:        0,
		CurrentEra:  0,
		PerfTier:    "background",
		NPCCount:    0,
		SaveVersion: currentSaveVersion,
		SaveState:   "normal",
		DataDir:     dataDir,
	}

	return wp
}

func (wp *WorldPersistence) EnsureDir() error {
	return os.MkdirAll(wp.dataDir, 0755)
}

func (wp *WorldPersistence) Metadata() WorldMetadata {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	return wp.meta
}

func (wp *WorldPersistence) SaveMetadata(tick uint32, era uint16, tier string, npcCount uint32, focusCount uint32) error {
	wp.mu.Lock()
	wp.meta.Tick = tick
	wp.meta.CurrentEra = era
	wp.meta.PerfTier = tier
	wp.meta.NPCCount = npcCount
	wp.meta.FocusCount = focusCount
	wp.meta.LastSavedAt = time.Now()
	meta := wp.meta
	wp.mu.Unlock()

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := wp.metaPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, wp.metaPath)
}

func (wp *WorldPersistence) SetSaveState(state string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.meta.SaveState = state
}

func (wp *WorldPersistence) DataDir() string {
	return wp.dataDir
}

func (wp *WorldPersistence) HasExistingSave() bool {
	_, err := os.Stat(wp.metaPath)
	return err == nil
}

func (wp *WorldPersistence) EstimateSizeBytes() int64 {
	var total int64

	filepath.Walk(wp.dataDir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})

	return total
}

func (wp *WorldPersistence) AvailableSpaceBytes() (int64, error) {
	dir := wp.dataDir
	if dir == "" {
		dir = "."
	}

	fs := getFilesystemStats(dir)
	return fs.AvailableBytes, nil
}

type filesystemStats struct {
	TotalBytes     int64
	AvailableBytes int64
	FreeBytes      int64
}

func getFilesystemStats(path string) filesystemStats {
	var stats filesystemStats
	stats = getFilesystemStatsFromSys(path)
	if stats.TotalBytes > 0 {
		return stats
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		if entries, err := os.ReadDir(path); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if fi, err := entry.Info(); err == nil {
					stats.TotalBytes += fi.Size()
				}
			}
		}
	}

	stats.AvailableBytes = 10 * 1024 * 1024 * 1024
	stats.FreeBytes = stats.AvailableBytes

	return stats
}

func getFilesystemStatsFromSys(path string) filesystemStats {
	var stats filesystemStats
	fillFilesystemStats(path, &stats)
	return stats
}

func writeJSONAtomic(path string, data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func readJSON(path string, out interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (wp *WorldPersistence) CreatePlaceholder() error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.placeholderCreated {
		return nil
	}

	path := wp.globalPlaceholderPath
	if fileExists(path) {
		fi, err := os.Stat(path)
		if err == nil {
			wp.placeholderSize = fi.Size()
			wp.placeholderCreated = true
			return nil
		}
	}

	fs := getFilesystemStats(wp.dataDir)
	size, ok := calcPlaceholderSize(fs.TotalBytes, fs.AvailableBytes)
	if !ok {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 64*1024)
	remaining := size
	for remaining > 0 {
		writeSize := int64(len(buf))
		if remaining < writeSize {
			writeSize = remaining
		}
		if _, err := f.Write(buf[:writeSize]); err != nil {
			f.Close()
			os.Remove(path)
			return err
		}
		remaining -= writeSize
	}

	if err := f.Sync(); err != nil {
		return err
	}

	wp.placeholderSize = size
	wp.placeholderCreated = true
	return nil
}

func (wp *WorldPersistence) ReleasePlaceholder() error {
	wp.mu.Lock()
	path := wp.globalPlaceholderPath
	wp.mu.Unlock()

	if !fileExists(path) {
		return nil
	}
	err := os.Remove(path)
	if err == nil {
		wp.mu.Lock()
		wp.placeholderCreated = false
		wp.placeholderSize = 0
		wp.mu.Unlock()
	}
	return err
}

func (wp *WorldPersistence) HasPlaceholder() bool {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	if wp.placeholderCreated {
		return true
	}
	return fileExists(wp.globalPlaceholderPath)
}

func (wp *WorldPersistence) PlaceholderSize() int64 {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	return wp.placeholderSize
}

type StorageStatus struct {
	Level         string `json:"level"`
	TotalBytes    int64  `json:"total_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
	UsedBytes     int64  `json:"used_bytes"`
	PlaceholderActive bool `json:"placeholder_active"`
	PlaceholderSize int64 `json:"placeholder_size"`
}

func (wp *WorldPersistence) GetStorageStatus() StorageStatus {
	fs := getFilesystemStats(wp.dataDir)
	used := wp.EstimateSizeBytes()

	level := StorageLevelNormal
	avail := fs.AvailableBytes
	if avail < StorageCriticalBytes {
		level = StorageLevelCritical
	} else if avail < StorageLowBytes {
		level = StorageLevelLow
	}

	return StorageStatus{
		Level:             level,
		TotalBytes:        fs.TotalBytes,
		AvailableBytes:    avail,
		UsedBytes:         used,
		PlaceholderActive: wp.HasPlaceholder(),
		PlaceholderSize:   wp.placeholderSize,
	}
}

func (wp *WorldPersistence) CheckStorageAndAdjust(world *FractalWorld) string {
	status := wp.GetStorageStatus()

	switch status.Level {
	case StorageLevelCritical:
		if status.PlaceholderActive {
			wp.ReleasePlaceholder()
			return "storage_critical_released_placeholder"
		}
		return "storage_critical"
	case StorageLevelLow:
		wp.SetSaveState("low_space")
		return "storage_low"
	default:
		wp.SetSaveState("normal")
		return "normal"
	}
}
