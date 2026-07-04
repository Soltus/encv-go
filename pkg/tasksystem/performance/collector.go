package performance

import (
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const maxSamples = 64

// phaseFrame 表示一个正在执行的阶段的栈帧。
// Bytes 字段通过 sync/atomic 原子累加，供 RecordBytes 在不加锁的情况下更新。
type phaseFrame struct {
	Phase     string
	StartedAt time.Time
	Bytes     int64 // atomic
}

// throughputSample 是吞吐量环形缓冲中的一个采样点。
type throughputSample struct {
	Timestamp time.Time
	Bytes     int64 // 累计字节数快照
}

// Collector 轻量级性能采集器。
// 热路径（RecordBytes）仅使用 sync/atomic + 极少 mutex 竞争，开销 <1%。
type Collector struct {
	taskID    string
	taskType  string
	startTime time.Time

	mu              sync.Mutex
	phaseStack      []*phaseFrame
	completedPhases []PhaseTiming

	// currentPhase 指向栈顶 phaseFrame，供 RecordBytes 无锁读取。
	currentPhase atomic.Pointer[phaseFrame]

	// 环形缓冲（容量 maxSamples=64）
	throughputSamples []throughputSample
	sampleHead        int

	// 采样状态（原子读写，快速路径无需 mutex）
	lastSampleTimeNs atomic.Int64
	lastSampleBytes  atomic.Int64

	totalBytes int64 // atomic
}

// NewCollector 创建一个新的采集器。
func NewCollector(taskID, taskType string) *Collector {
	now := time.Now()
	c := &Collector{
		taskID:            taskID,
		taskType:          taskType,
		startTime:         now,
		throughputSamples: make([]throughputSample, 0, maxSamples),
	}
	c.lastSampleTimeNs.Store(now.UnixNano())
	c.lastSampleBytes.Store(0)
	// 写入基线采样点（time=start, bytes=0），保证后续吞吐量计算有起点
	c.addSample(throughputSample{Timestamp: now, Bytes: 0})
	return c
}

// StartPhase 压入一个新的阶段栈帧。
func (c *Collector) StartPhase(phase string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	frame := &phaseFrame{
		Phase:     phase,
		StartedAt: time.Now(),
	}
	c.phaseStack = append(c.phaseStack, frame)
	c.currentPhase.Store(frame)
}

// EndPhase 弹出栈顶阶段，计算 DurationMs 并追加到 completedPhases。
// bytesProcessed > 0 时使用调用方提供的值；为 0 时回退到 phaseFrame 累计值。
func (c *Collector) EndPhase(phase string, bytesProcessed int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.phaseStack) == 0 {
		return
	}
	frame := c.phaseStack[len(c.phaseStack)-1]
	c.phaseStack = c.phaseStack[:len(c.phaseStack)-1]

	if len(c.phaseStack) > 0 {
		c.currentPhase.Store(c.phaseStack[len(c.phaseStack)-1])
	} else {
		c.currentPhase.Store(nil)
	}

	duration := time.Since(frame.StartedAt)
	durationMs := duration.Milliseconds()

	effectiveBytes := bytesProcessed
	if effectiveBytes == 0 {
		effectiveBytes = atomic.LoadInt64(&frame.Bytes)
	}

	var throughputMBps float64
	if duration.Seconds() > 0 && effectiveBytes > 0 {
		throughputMBps = float64(effectiveBytes) / duration.Seconds() / 1024 / 1024
	}

	c.completedPhases = append(c.completedPhases, PhaseTiming{
		Phase:          frame.Phase,
		DurationMs:     durationMs,
		BytesProcessed: effectiveBytes,
		ThroughputMBps: throughputMBps,
	})
}

// RecordBytes 原子累加 totalBytes 与当前阶段 bytes，并按 500ms/1MB 阈值采样。
// 快速路径（未达采样阈值）仅使用 atomic 操作，不获取 mutex。
func (c *Collector) RecordBytes(n int64) {
	if n <= 0 {
		return
	}
	total := atomic.AddInt64(&c.totalBytes, n)

	if phase := c.currentPhase.Load(); phase != nil {
		atomic.AddInt64(&phase.Bytes, n)
	}

	now := time.Now()
	lastTimeNs := c.lastSampleTimeNs.Load()
	lastBytes := c.lastSampleBytes.Load()

	elapsedNs := now.UnixNano() - lastTimeNs
	bytesDiff := total - lastBytes

	if elapsedNs >= int64(500*time.Millisecond) || bytesDiff >= 1024*1024 {
		c.mu.Lock()
		// 双重检查，避免并发重复采样
		lastTimeNs = c.lastSampleTimeNs.Load()
		lastBytes = c.lastSampleBytes.Load()
		elapsedNs = now.UnixNano() - lastTimeNs
		bytesDiff = total - lastBytes

		if elapsedNs >= int64(500*time.Millisecond) || bytesDiff >= 1024*1024 {
			c.addSample(throughputSample{Timestamp: now, Bytes: total})
			c.lastSampleTimeNs.Store(now.UnixNano())
			c.lastSampleBytes.Store(total)
		}
		c.mu.Unlock()
	}
}

// addSample 向环形缓冲写入一个采样点。调用方须持有 c.mu。
func (c *Collector) addSample(sample throughputSample) {
	if len(c.throughputSamples) < maxSamples {
		c.throughputSamples = append(c.throughputSamples, sample)
	} else {
		c.throughputSamples[c.sampleHead] = sample
		c.sampleHead = (c.sampleHead + 1) % maxSamples
	}
}

// getSamples 按时间顺序返回所有采样点。调用方须持有 c.mu。
func (c *Collector) getSamples() []throughputSample {
	if len(c.throughputSamples) < maxSamples {
		result := make([]throughputSample, len(c.throughputSamples))
		copy(result, c.throughputSamples)
		return result
	}
	result := make([]throughputSample, maxSamples)
	for i := 0; i < maxSamples; i++ {
		result[i] = c.throughputSamples[(c.sampleHead+i)%maxSamples]
	}
	return result
}

// WrapReader 包装一个 io.Reader，Read 时自动调 RecordBytes。
func (c *Collector) WrapReader(r io.Reader) io.Reader {
	return &readerWrapper{reader: r, collector: c}
}

// WrapWriter 包装一个 io.Writer，Write 时自动调 RecordBytes。
func (c *Collector) WrapWriter(w io.Writer) io.Writer {
	return &writerWrapper{writer: w, collector: c}
}

type readerWrapper struct {
	reader    io.Reader
	collector *Collector
}

func (rw *readerWrapper) Read(p []byte) (int, error) {
	n, err := rw.reader.Read(p)
	if n > 0 {
		rw.collector.RecordBytes(int64(n))
	}
	return n, err
}

type writerWrapper struct {
	writer    io.Writer
	collector *Collector
}

func (ww *writerWrapper) Write(p []byte) (int, error) {
	n, err := ww.writer.Write(p)
	if n > 0 {
		ww.collector.RecordBytes(int64(n))
	}
	return n, err
}

// Finalize 计算并返回最终性能指标。调用后不应再使用该 Collector。
func (c *Collector) Finalize(sourceSize, outputSize int64, cpuScore float64, cpuLabel string) PerformanceMetrics {
	c.mu.Lock()
	// 自动结束尚未 EndPhase 的阶段
	for len(c.phaseStack) > 0 {
		frame := c.phaseStack[len(c.phaseStack)-1]
		c.phaseStack = c.phaseStack[:len(c.phaseStack)-1]

		duration := time.Since(frame.StartedAt)
		durationMs := duration.Milliseconds()
		effectiveBytes := atomic.LoadInt64(&frame.Bytes)

		var throughputMBps float64
		if duration.Seconds() > 0 && effectiveBytes > 0 {
			throughputMBps = float64(effectiveBytes) / duration.Seconds() / 1024 / 1024
		}

		c.completedPhases = append(c.completedPhases, PhaseTiming{
			Phase:          frame.Phase,
			DurationMs:     durationMs,
			BytesProcessed: effectiveBytes,
			ThroughputMBps: throughputMBps,
		})
	}

	// 写入最终采样点，捕获最后一段数据
	now := time.Now()
	totalBytes := atomic.LoadInt64(&c.totalBytes)
	c.addSample(throughputSample{Timestamp: now, Bytes: totalBytes})
	samples := c.getSamples()

	phaseTimings := make([]PhaseTiming, len(c.completedPhases))
	copy(phaseTimings, c.completedPhases)
	c.mu.Unlock()

	// 计算总耗时
	totalDuration := time.Since(c.startTime)
	totalDurationMs := totalDuration.Milliseconds()

	// 计算平均吞吐量 (MB/s)
	var avgThroughput float64
	if totalDuration.Seconds() > 0 && sourceSize > 0 {
		avgThroughput = float64(sourceSize) / totalDuration.Seconds() / 1024 / 1024
	}

	// 从采样点计算 Peak/P50/P99 (MB/s)
	var peakThroughput, p50Throughput, p99Throughput float64
	if len(samples) >= 2 {
		throughputs := make([]float64, 0, len(samples)-1)
		for i := 1; i < len(samples); i++ {
			bytesDiff := samples[i].Bytes - samples[i-1].Bytes
			timeDiff := samples[i].Timestamp.Sub(samples[i-1].Timestamp)
			if timeDiff.Seconds() > 0 && bytesDiff > 0 {
				mbps := float64(bytesDiff) / timeDiff.Seconds() / 1024 / 1024
				throughputs = append(throughputs, mbps)
			}
		}
		if len(throughputs) > 0 {
			sort.Float64s(throughputs)
			peakThroughput = throughputs[len(throughputs)-1]
			p50Throughput = throughputs[len(throughputs)/2]
			p99Throughput = throughputs[(len(throughputs)*99)/100]
		}
	}

	// 计算大小比率
	var sizeRatio float64
	if sourceSize > 0 {
		sizeRatio = float64(outputSize) / float64(sourceSize)
	}

	return PerformanceMetrics{
		TaskID:          c.taskID,
		TaskType:        c.taskType,
		SourceSize:      sourceSize,
		OutputSize:      outputSize,
		SizeRatio:       sizeRatio,
		AvgThroughput:   avgThroughput,
		PeakThroughput:  peakThroughput,
		P50Throughput:   p50Throughput,
		P99Throughput:   p99Throughput,
		PhaseTimings:    phaseTimings,
		TotalDurationMs: totalDurationMs,
		CPUScore:        cpuScore,
		CPULabel:        cpuLabel,
		CreatedAt:       time.Now(),
	}
}
