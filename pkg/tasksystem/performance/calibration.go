package performance

import (
	"bytes"
	"crypto/rand"
	"io"
	"runtime"
	"time"

	"github.com/Soltus/encv-go/internal/v2/crypto"
)

// RunCalibration 运行 AES-CTR 1MB 加密测试，计算 CPUScore。
// 耗时约 50-200ms，启动时调用一次。
func RunCalibration() CalibrationResult {
	// 1. 准备 1MB 随机数据
	dataSize := 1024 * 1024 // 1MB
	data := make([]byte, dataSize)
	rand.Read(data)

	// 2. 准备 32 字节 key + 16 字节 iv
	key := make([]byte, 32)
	iv := make([]byte, 16)
	rand.Read(key)
	rand.Read(iv)

	// 3. 跑 3 次取平均（第一次可能冷启动）
	var totalMBps float64
	runs := 3
	for i := 0; i < runs; i++ {
		src := bytes.NewReader(data)
		dst := io.Discard
		start := time.Now()
		err := crypto.EncryptStream_v2(src, dst, key, iv)
		elapsed := time.Since(start)
		if err != nil {
			continue // 跳过失败
		}
		seconds := elapsed.Seconds()
		if seconds > 0 {
			mbps := float64(dataSize) / 1024 / 1024 / seconds
			totalMBps += mbps
		}
	}
	aesThroughput := totalMBps / float64(runs)

	// 4. 计算 CPUScore（基准 3000 MB/s）
	cpuScore := aesThroughput / 3000.0

	// 5. 分级
	var cpuLabel string
	switch {
	case cpuScore >= 1.5:
		cpuLabel = "fast"
	case cpuScore >= 0.5:
		cpuLabel = "medium"
	default:
		cpuLabel = "slow"
	}

	return CalibrationResult{
		CPUScore:      cpuScore,
		AESThroughput: aesThroughput,
		CPULabel:      cpuLabel,
		CalibratedAt:  time.Now(),
		GoVersion:     runtime.Version(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		NumCPU:        runtime.NumCPU(),
	}
}
