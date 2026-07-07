package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Soltus/encv-go/pkg/tasksystem/performance"
	"github.com/Soltus/encv-go/pkg/tasksystem/store/sqlite"
	"github.com/pterm/pterm"
)

type benchResult struct {
	Name        string  `json:"name"`
	Package     string  `json:"package"`
	Layer       string  `json:"layer"`
	IterCount   int64   `json:"iterCount"`
	NsPerOp     float64 `json:"nsPerOp"`
	MBPerSec    float64 `json:"mbPerSec"`
	BytesPerOp  int64   `json:"bytesPerOp"`
	AllocsPerOp int64   `json:"allocsPerOp"`
	SubName     string  `json:"subName"`
}

// calibrationResult 硬件校准结果
type calibrationResult struct {
	CPUScore      float64 // 相对基准 CPU 的分数（1.0 = 基准线）
	AESThroughput float64 // AES-CTR 单线程吞吐量 MB/s
	CPULabel      string  // 性能标签：fast/medium/slow
}

type jsonTestEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

type packageConfig struct {
	Dir         string
	Layer       string
	Description string
	BenchArgs   []string
	BuildTags   string
}

type reportData struct {
	Timestamp      string
	GoVersion      string
	OS             string
	Arch           string
	CPU            string
	Results        []benchResult
	Layers         []layerInfo
	CategoryGroups []categoryGroup
	OverallScore   float64
	Insights       []insightItem
	TotalTime      string
	PassCount      int
	HasHistory     bool
	HistoryTime    string
	HistoryCount   int
	HistoryResults []benchResult
	Calibration    calibrationResult
}

type layerInfo struct {
	Name        string
	Description string
	Results     []benchResult
}

// benchDesc 描述一个基准测试项的人可读信息
type benchDesc struct {
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Unit        string  `json:"unit"`
	GoodMB      float64 `json:"goodMB,omitempty"`
	ExcelMB     float64 `json:"excelMB,omitempty"`
	GoodNs      float64 `json:"goodNs,omitempty"`
	ExcelNs     float64 `json:"excelNs,omitempty"`
	Note        string  `json:"note,omitempty"`
}

// historyFile 历史缓存文件结构
type historyFile struct {
	Timestamp string        `json:"timestamp"`
	CPU       string        `json:"cpu"`
	OS        string        `json:"os"`
	Arch      string        `json:"arch"`
	GoVersion string        `json:"goVersion"`
	Results   []benchResult `json:"results"`
}

type categoryDef struct {
	ID            string
	Name          string
	Description   string
	BenchPrefixes []string
}

type categoryGroup struct {
	ID          string
	Name        string
	Description string
	Results     []benchResult
	Score       float64
	ExcelCount  int
	GoodCount   int
	WarnCount   int
}

type insightItem struct {
	Type    string
	Message string
}

var benchLineRe = regexp.MustCompile(`^(Benchmark\S+)-\d+\s+(\d+)\s+(\d+(?:\.\d+)?)\s+ns/op(?:\s+([\d.]+)\s+MB/s)?(?:\s+(\d+)\s+B/op)?(?:\s+(\d+)\s+allocs/op)?`)

var benchNameLineRe = regexp.MustCompile(`^(Benchmark\S+-\d+)\s*$`)
var benchValueLineRe = regexp.MustCompile(`^\s+(\d+)\s+(\d+(?:\.\d+)?)\s+ns/op(?:\s+([\d.]+)\s+MB/s)?(?:\s+(\d+)\s+B/op)?(?:\s+(\d+)\s+allocs/op)?`)

var packages = []packageConfig{
	{Dir: "internal/v2/crypto", Layer: "L1", Description: "密码学原语"},
	{Dir: "internal/v2/container/envelope", Layer: "L2", Description: "信封解析"},
	{Dir: "internal/v2/container/manifest", Layer: "L2", Description: "清单序列化"},
	{Dir: "internal/v2/container/block", Layer: "L2", Description: "容器 Block I/O"},
	{Dir: "internal/v2/container/fragment", Layer: "L2", Description: "分片管理"},
	{Dir: "internal/v2/container/detector", Layer: "L2", Description: "容器检测"},
	{Dir: "internal/v2/physical", Layer: "L2", Description: "物理打包/解包"},
	{Dir: "internal/v2/reader", Layer: "L3", Description: "解密读取器"},
	{Dir: "internal/v2/bench", Layer: "L4/L5", Description: "端到端集成", BuildTags: "integration"},
}

// benchDescriptions 为每个基准测试项提供人可读描述和评级标准
// GoodMB/ExcelMB: 吞吐量及格/优秀线 (MB/s)
// GoodNs/ExcelNs: 延迟及格/优秀线 (ns/op)，仅对无吞吐量的测试项生效
var benchDescriptions = map[string]benchDesc{
	"GenerateKey_v2": {
		Category:    "密钥派生",
		Description: "使用 PBKDF2 从密码派生 256 位加密密钥，含盐值生成",
		Unit:        "延迟",
		GoodNs:      20_000_000,
		ExcelNs:     10_000_000,
		Note:        "PBKDF2 迭代次数决定耗时，安全性与速度的权衡",
	},
	"GenerateSalt_v2": {
		Category:    "密钥派生",
		Description: "生成加密盐值，使用 crypto/rand 安全随机源",
		Unit:        "延迟",
		GoodNs:      50_000,
		ExcelNs:     10_000,
	},
	"GenerateIV_v2": {
		Category:    "密钥派生",
		Description: "生成 AES-CTR 初始向量 (IV)，16 字节随机数",
		Unit:        "延迟",
		GoodNs:      50_000,
		ExcelNs:     10_000,
	},
	"EncryptStream_v2": {
		Category:    "AES-CTR 加密",
		Description: "流式 AES-256-CTR 加密，数据通过 io.Reader 逐块处理",
		Unit:        "吞吐量",
		GoodMB:      1000,
		ExcelMB:     3000,
		Note:        "CTR 模式理论可并行，实际受内存带宽限制",
	},
	"DecryptStream_v2": {
		Category:    "AES-CTR 解密",
		Description: "流式 AES-256-CTR 解密，与加密对称",
		Unit:        "吞吐量",
		GoodMB:      1000,
		ExcelMB:     3000,
		Note:        "解密通常与加密同速，CTR 模式加解密一致",
	},
	"EncryptBytes_v2": {
		Category:    "AES-CTR 加密",
		Description: "内存中一次性 AES-256-CTR 加密，适合小数据",
		Unit:        "吞吐量",
		GoodMB:      1000,
		ExcelMB:     4000,
	},
	"DecryptBytes_v2": {
		Category:    "AES-CTR 解密",
		Description: "内存中一次性 AES-256-CTR 解密，适合小数据",
		Unit:        "吞吐量",
		GoodMB:      1000,
		ExcelMB:     4000,
	},
	"DeriveCTRIVForOffset_v2": {
		Category:    "CTR IV 推导",
		Description: "根据文件偏移量推导 CTR 模式 IV，实现 Seek 的关键",
		Unit:        "延迟",
		GoodNs:      5_000,
		ExcelNs:     1_000,
		Note:        "O(1) 复杂度，偏移量不影响耗时",
	},
	"EncryptSystemPayload": {
		Category:    "系统负载加密",
		Description: "加密容器系统元数据块（索引、配置等）",
		Unit:        "吞吐量",
		GoodMB:      500,
		ExcelMB:     2000,
	},
	"DecryptSystemPayload": {
		Category:    "系统负载解密",
		Description: "解密容器系统元数据块",
		Unit:        "吞吐量",
		GoodMB:      500,
		ExcelMB:     2000,
	},
	"EncryptToTempFile_v2": {
		Category:    "文件加密",
		Description: "加密数据并写入临时文件，涉及磁盘 I/O",
		Unit:        "吞吐量",
		GoodMB:      100,
		ExcelMB:     500,
		Note:        "受磁盘写入速度影响，SSD 通常 500MB/s+",
	},
	"WriteBlock_v2": {
		Category:    "容器写入",
		Description: "将加密数据块写入容器文件，含头部序列化",
		Unit:        "吞吐量",
		GoodMB:      1000,
		ExcelMB:     3000,
	},
	"ReadBlockHeader_v2": {
		Category:    "容器读取",
		Description: "读取容器块头部，解析元数据索引",
		Unit:        "延迟",
		GoodNs:      50_000,
		ExcelNs:     10_000,
	},
	"ReadBlockData_v2": {
		Category:    "容器读取",
		Description: "读取容器块数据并解密",
		Unit:        "吞吐量",
		GoodMB:      2000,
		ExcelMB:     5000,
	},
	"CalculateFragmentSize": {
		Category:    "分片计算",
		Description: "根据文件大小和物理偏移计算逻辑分片尺寸",
		Unit:        "延迟",
		GoodNs:      100_000,
		ExcelNs:     10_000,
		Note:        "50GB/0B 特例会触发全量扫描，属极端情况",
	},
	"CreateLogicalFragmentsFromSize": {
		Category:    "分片构建",
		Description: "根据文件大小创建逻辑分片列表，用于虚拟 Seek",
		Unit:        "延迟",
		GoodNs:      100_000,
		ExcelNs:     20_000,
	},
	"CreateLogicalFragmentsFromSizeAligned": {
		Category:    "分片构建",
		Description: "创建对齐的逻辑分片，含物理/逻辑偏移映射",
		Unit:        "延迟",
		GoodNs:      100_000,
		ExcelNs:     20_000,
	},
	"ValidateGlobalStartOffsets": {
		Category:    "分片校验",
		Description: "校验所有分片全局起始偏移的一致性",
		Unit:        "延迟",
		GoodNs:      100_000,
		ExcelNs:     30_000,
	},
	"DecryptReaderFactory_ParseAndCache": {
		Category:    "读取器初始化",
		Description: "解析容器索引并缓存解密上下文，首次打开文件时执行",
		Unit:        "延迟",
		GoodNs:      50_000_000,
		ExcelNs:     15_000_000,
		Note:        "一次性开销，后续读取复用缓存",
	},
	"DecryptReaderFactory_NewDecryptReader": {
		Category:    "读取器创建",
		Description: "创建新的解密读取器实例，含密钥准备",
		Unit:        "延迟",
		GoodNs:      50_000_000,
		ExcelNs:     15_000_000,
	},
	"SequentialDecryptReader_Read": {
		Category:    "顺序解密读取",
		Description: "顺序读取并解密数据，适合完整播放场景",
		Unit:        "吞吐量",
		GoodMB:      100,
		ExcelMB:     500,
		Note:        "流式视频播放的典型路径",
	},
	"SequentialSeekableDecryptReader_Read": {
		Category:    "可 Seek 顺序读取",
		Description: "支持 Seek 的顺序解密读取，分片数影响性能",
		Unit:        "吞吐量",
		GoodMB:      50,
		ExcelMB:     300,
		Note:        "分片越多性能越低，100 分片时显著下降",
	},
	"VirtualSeekableDecryptReader_Seek": {
		Category:    "虚拟 Seek",
		Description: "虚拟 Seekable 读取器的跳转操作，不预读数据",
		Unit:        "延迟",
		GoodNs:      1_000_000,
		ExcelNs:     100_000,
		Note:        "跳转到 25%/75% 需要跨分片，比 head/tail 慢",
	},
	"SequentialSeekableDecryptReader_Seek": {
		Category:    "顺序 Seek",
		Description: "顺序 Seekable 读取器的跳转操作，需重建读取状态",
		Unit:        "延迟",
		GoodNs:      5_000_000,
		ExcelNs:     500_000,
		Note:        "向后 Seek 需要丢弃数据，比虚拟 Seek 慢",
	},
	"BulkDecryptor_DecryptToFile": {
		Category:    "批量解密",
		Description: "将加密文件完整解密到目标路径，含磁盘 I/O",
		Unit:        "吞吐量",
		GoodMB:      100,
		ExcelMB:     500,
		Note:        "受磁盘读写速度双重影响",
	},
	"ParseEnvelopeFooterFromBytes": {
		Category:    "信封解析",
		Description: "从字节数组解析容器 Footer，判断文件是否为 ENCV 容器",
		Unit:        "延迟",
		GoodNs:      5_000,
		ExcelNs:     1_000,
		Note:        "网络流场景的关键路径，每次请求都需解析",
	},
	"ReadEnvelopeFooter_v2": {
		Category:    "信封解析",
		Description: "从文件读取并解析容器 Footer，含 Seek 操作",
		Unit:        "延迟",
		GoodNs:      50_000,
		ExcelNs:     10_000,
	},
	"IsEncvContainerFromBytes": {
		Category:    "容器检测",
		Description: "从字节数组快速判断是否为 ENCV 容器",
		Unit:        "延迟",
		GoodNs:      5_000,
		ExcelNs:     1_000,
		Note:        "代理服务器的热路径，每个请求都需判断",
	},
	"DetectContainer": {
		Category:    "容器检测",
		Description: "完整容器检测：解析 Footer + 读取 Manifest + 判断可寻址性",
		Unit:        "延迟",
		GoodNs:      100_000_000,
		ExcelNs:     30_000_000,
		Note:        "首次打开文件的完整检测流程，含磁盘 I/O",
	},
	"DetectIndexKind": {
		Category:    "容器检测",
		Description: "检测容器索引类型（video/archive 等），需读取 Manifest",
		Unit:        "延迟",
		GoodNs:      100_000_000,
		ExcelNs:     30_000_000,
	},
	"SerializeManifest_v2": {
		Category:    "清单序列化",
		Description: "将 Manifest 序列化为 JSON，分片数影响耗时",
		Unit:        "延迟",
		GoodNs:      500_000,
		ExcelNs:     100_000,
		Note:        "加密流程的最后一步，分片数越多越慢",
	},
	"DeserializeManifest_v2": {
		Category:    "清单序列化",
		Description: "从 JSON 反序列化 Manifest，含分片列表解析",
		Unit:        "延迟",
		GoodNs:      500_000,
		ExcelNs:     100_000,
	},
	"EncryptManifest": {
		Category:    "清单加密",
		Description: "加密序列化后的 Manifest 数据",
		Unit:        "吞吐量",
		GoodMB:      500,
		ExcelMB:     2000,
	},
	"DecryptManifest": {
		Category:    "清单解密",
		Description: "解密加密的 Manifest 数据",
		Unit:        "吞吐量",
		GoodMB:      500,
		ExcelMB:     2000,
	},
	"SinglePhysicalPacker_Pack": {
		Category:    "物理打包",
		Description: "将加密数据打包为单文件容器，含 Block 写入和 Footer 生成",
		Unit:        "吞吐量",
		GoodMB:      100,
		ExcelMB:     500,
		Note:        "受磁盘写入速度影响",
	},
	"SinglePhysicalUnpacker_Unpack": {
		Category:    "物理解包",
		Description: "单文件容器解包（直接返回路径，零开销）",
		Unit:        "延迟",
		GoodNs:      100_000,
		ExcelNs:     10_000,
		Note:        "单文件模式无需重建，几乎零开销",
	},
	"FileChunkerPhysicalPacker_Pack": {
		Category:    "分片打包",
		Description: "将加密数据分片打包为多个物理文件，含分片切换和 CRC 计算",
		Unit:        "吞吐量",
		GoodMB:      50,
		ExcelMB:     200,
		Note:        "分片越多 I/O 越复杂，适合超大文件场景",
	},
	"FileChunkerPhysicalUnpacker_Unpack": {
		Category:    "分片解包",
		Description: "将多分片容器重建为单文件，含数据搬运和 Footer 重写",
		Unit:        "吞吐量",
		GoodMB:      50,
		ExcelMB:     200,
		Note:        "OpenList 代理的关键路径，需将分片合并为单文件",
	},
	"VirtualSeekableDecryptReader_SeekMatrix": {
		Category:    "Seek 矩阵",
		Description: "不同分片数下的虚拟 Seek 性能，量化分片数对跳转延迟的影响",
		Unit:        "延迟",
		GoodNs:      1_000_000,
		ExcelNs:     100_000,
		Note:        "分片数越多，定位目标分片越慢",
	},
	"SequentialSeekableDecryptReader_SeekMatrix": {
		Category:    "Seek 矩阵",
		Description: "不同分片数下的顺序 Seek 性能，需重建读取状态",
		Unit:        "延迟",
		GoodNs:      5_000_000,
		ExcelNs:     500_000,
	},
	"ConcurrentDecryptReader": {
		Category:    "并发读取",
		Description: "多 goroutine 并发解密读取，测试资源竞争和并发安全",
		Unit:        "吞吐量",
		GoodMB:      200,
		ExcelMB:     800,
		Note:        "多用户同时播放场景，受文件句柄和锁竞争影响",
	},
	"ConcurrentSeekRead": {
		Category:    "并发 Seek",
		Description: "多 goroutine 并发 Seek+读取，测试随机访问并发能力",
		Unit:        "延迟",
		GoodNs:      10_000_000,
		ExcelNs:     2_000_000,
		Note:        "视频拖拽场景，多个用户同时跳转",
	},
	"DeriveKEK": {
		Category:    "分层密钥",
		Description: "使用 PBKDF2-SHA256 从密码派生 32 字节 KEK（密钥加密密钥），100,000 次迭代",
		Unit:        "延迟",
		GoodNs:      20_000_000,
		ExcelNs:     10_000_000,
		Note:        "KEK 仅用于加密 DEK，不直接加密数据",
	},
	"GenerateDEK": {
		Category:    "分层密钥",
		Description: "生成随机 16 字节 DEK（数据加密密钥），使用 crypto/rand 安全随机源",
		Unit:        "延迟",
		GoodNs:      50_000,
		ExcelNs:     10_000,
	},
	"WrapDEK": {
		Category:    "分层密钥",
		Description: "使用 AES-256-GCM 用 KEK 包裹 DEK，提供认证加密",
		Unit:        "延迟",
		GoodNs:      500_000,
		ExcelNs:     100_000,
		Note:        "GCM 提供完整性和认证，防止 DEK 被篡改",
	},
	"UnwrapDEK": {
		Category:    "分层密钥",
		Description: "使用 AES-256-GCM 用 KEK 解包 DEK，验证密码正确性",
		Unit:        "延迟",
		GoodNs:      500_000,
		ExcelNs:     100_000,
		Note:        "密码错误时 GCM 认证失败，返回 ErrWrongPassword",
	},
	"LargeFileSimulate_Encrypt": {
		Category:    "大文件模拟",
		Description: "大文件加密吞吐量模拟（零内存占用），测试纯算法性能上限",
		Unit:        "吞吐量",
		GoodMB:      1000,
		ExcelMB:     3000,
		Note:        "使用 zero Reader 模拟，无 I/O 干扰，反映 CPU 加密上限",
	},
	"LargeFileSimulate_Decrypt": {
		Category:    "大文件模拟",
		Description: "大文件解密吞吐量模拟（零内存占用），与加密对称",
		Unit:        "吞吐量",
		GoodMB:      1000,
		ExcelMB:     3000,
	},
	"AlistEncrypt_Compare": {
		Category:    "插件性能",
		Description: "alistencrypt 插件对比基准，作为性能基线参考",
		Unit:        "吞吐量",
		GoodMB:      500,
		ExcelMB:     2000,
		Note:        "alistencrypt 使用 AES-128-CTR + PBKDF2 1000 iter + 无 MAC",
	},
	"CryptoCore_Throughput": {
		Category:    "加密核心",
		Description: "加密核心吞吐量对比（AES-128 vs AES-256、内存 vs 流式）",
		Unit:        "吞吐量",
		GoodMB:      1500,
		ExcelMB:     4000,
		Note:        "纯内存操作，无 I/O 干扰，反映算法理论峰值",
	},
	"PhysicalChunking_LargeFile": {
		Category:    "大文件模拟",
		Description: "大文件物理分片索引构建性能（纯计算，无 I/O）",
		Unit:        "延迟",
		GoodNs:      1_000_000,
		ExcelNs:     100_000,
		Note:        "分片数越多，索引构建开销越大",
	},
}

var categoryDefs = []categoryDef{
	{
		ID: "key-derivation", Name: "密钥派生",
		Description:   "密码学密钥生成与派生，是加密系统的第一步。PBKDF2 迭代次数决定密钥强度与耗时",
		BenchPrefixes: []string{"GenerateKey", "GenerateSalt", "GenerateIV"},
	},
	{
		ID: "hierarchical-keys", Name: "分层密钥",
		Description:   "信封加密架构：DEK 随机生成加密数据，KEK 由密码派生加密 DEK，支持密码更换不重加密数据",
		BenchPrefixes: []string{"DeriveKEK", "GenerateDEK", "WrapDEK", "UnwrapDEK", "WrapUnwrapDEK"},
	},
	{
		ID: "aes-ctr", Name: "AES-CTR 加解密",
		Description:   "AES-128/256-CTR 流式加解密核心，支持 AES-NI 硬件加速时吞吐量可达数 GB/s",
		BenchPrefixes: []string{"EncryptStream", "DecryptStream", "EncryptBytes", "DecryptBytes", "DeriveCTRIV"},
	},
	{
		ID: "container-io", Name: "容器 Block I/O",
		Description:   "加密容器格式读写，包含头部序列化、数据块管理与临时文件加密",
		BenchPrefixes: []string{"EncryptSystemPayload", "DecryptSystemPayload", "WriteBlock", "ReadBlockHeader", "ReadBlockData", "EncryptToTempFile"},
	},
	{
		ID: "envelope", Name: "信封与检测",
		Description:   "容器信封解析与格式检测，是每次打开加密文件的第一步，直接影响首次访问延迟",
		BenchPrefixes: []string{"ParseEnvelope", "ReadEnvelope", "IsEncvContainer", "DetectContainer", "DetectIndexKind"},
	},
	{
		ID: "manifest", Name: "清单序列化",
		Description:   "Manifest JSON 序列化/反序列化与加解密，分片数直接影响耗时",
		BenchPrefixes: []string{"SerializeManifest", "DeserializeManifest", "EncryptManifest", "DecryptManifest"},
	},
	{
		ID: "fragment", Name: "分片管理",
		Description:   "视频分片索引构建与校验，为虚拟 Seek 和 GOP 对齐提供映射",
		BenchPrefixes: []string{"CalculateFragmentSize", "CreateLogicalFragments", "ValidateGlobalStartOffsets"},
	},
	{
		ID: "physical", Name: "物理打包/解包",
		Description:   "加密数据的物理存储管理：单文件打包、分片打包、分片重建，直接影响加密和解密速度",
		BenchPrefixes: []string{"SinglePhysical", "FileChunkerPhysical"},
	},
	{
		ID: "decrypt-reader", Name: "解密读取器",
		Description:   "视频播放核心路径：顺序读取、Seek 跳转、批量解密，直接影响播放体验",
		BenchPrefixes: []string{"DecryptReaderFactory", "SequentialDecryptReader", "VirtualSeekable", "SequentialSeekable", "BulkDecryptor"},
	},
	{
		ID: "plugins", Name: "插件性能",
		Description:   "各类型插件（video/audio/image/pdf/text/wps）的端到端加密/解密/往返性能对比",
		BenchPrefixes: []string{"AllPlugins_Encrypt", "AllPlugins_Decrypt", "AllPlugins_RoundTrip", "AlistEncrypt_Compare"},
	},
	{
		ID: "large-file", Name: "大文件模拟",
		Description:   "大文件等效模拟性能（1GB/10GB/100GB），使用零 Reader 零内存占用，测试纯加密吞吐量上限",
		BenchPrefixes: []string{"LargeFileSimulate_Encrypt", "LargeFileSimulate_Decrypt", "PhysicalChunking_LargeFile"},
	},
	{
		ID: "crypto-core", Name: "加密核心",
		Description:   "加密核心吞吐量对比（AES-128 vs AES-256、内存 vs 流式），排除 I/O 干扰的纯算法性能",
		BenchPrefixes: []string{"CryptoCore_Throughput", "CryptoCore_WrapUnwrapDEK"},
	},
	{
		ID: "concurrency", Name: "并发与 Seek 矩阵",
		Description:   "多用户并发读取和不同分片数下的 Seek 性能，衡量生产环境下的真实表现",
		BenchPrefixes: []string{"ConcurrentDecryptReader", "ConcurrentSeekRead", "SeekMatrix"},
	},
}

// getBenchDesc 根据基准名称匹配描述
func getBenchDesc(name string) benchDesc {
	// 去掉 "Benchmark" 前缀
	base := strings.TrimPrefix(name, "Benchmark")

	// 处理子测试：先尝试匹配完整路径中的最后一段（子测试名）
	parts := strings.Split(base, "/")
	for i := len(parts); i > 0; i-- {
		candidate := strings.Join(parts[:i], "/")
		candidate = strings.TrimSuffix(candidate, "_v2")
		if desc, ok := benchDescriptions[candidate]; ok {
			return desc
		}
	}

	// 处理 CryptoCore_WrapUnwrapDEK 这类包含多个子测试的情况
	if strings.Contains(base, "WrapUnwrapDEK") && len(parts) >= 2 {
		lastPart := parts[len(parts)-1]
		if desc, ok := benchDescriptions[lastPart]; ok {
			return desc
		}
	}

	// 处理 AllPlugins_*/plugin/size/Op 格式
	if strings.Contains(base, "AllPlugins_") && len(parts) >= 3 {
		pluginName := parts[1]
		opName := parts[len(parts)-1]
		key := fmt.Sprintf("%s/%s", pluginName, opName)
		if desc, ok := benchDescriptions[key]; ok {
			return desc
		}
	}

	// 去掉 _v2 后缀尝试匹配（顶层）
	rootBase := parts[0]
	for _, key := range []string{rootBase, strings.TrimSuffix(rootBase, "_v2")} {
		if desc, ok := benchDescriptions[key]; ok {
			return desc
		}
	}

	return benchDesc{Category: "其他", Description: "基准测试项", Unit: "延迟"}
}

// getGrade 根据描述中的标准返回评级
func getGrade(r benchResult, desc benchDesc) string {
	if desc.Unit == "吞吐量" && r.MBPerSec > 0 {
		if desc.ExcelMB > 0 && r.MBPerSec >= desc.ExcelMB {
			return "excellent"
		}
		if desc.GoodMB > 0 && r.MBPerSec >= desc.GoodMB {
			return "good"
		}
		return "warn"
	}
	if desc.Unit == "延迟" {
		if desc.ExcelNs > 0 && r.NsPerOp <= desc.ExcelNs {
			return "excellent"
		}
		if desc.GoodNs > 0 && r.NsPerOp <= desc.GoodNs {
			return "good"
		}
		return "warn"
	}
	return ""
}

// formatElapsed 格式化总耗时，避免 Go 默认的 "3m3.9s" 等不直观格式
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// getMemoryInfoMB 获取系统内存信息（跨平台版本）
// 返回：可用 MB（基于 Go 进程视角）, 总计 MB, 可用 commit MB
// 注意：runtime.MemStats 只能获取 Go 进程视角的内存，无法获取系统级可用内存。
// 在非 Windows 平台，我们用 SysMem（系统内存总量）作为估算基础。
func getMemoryInfoMB() (availMB, totalMB, availCommitMB uint64) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	// Sys 是 Go 从系统获取的内存总量，作为 totalMB 的近似
	totalMB = ms.Sys / (1024 * 1024)
	// 估算可用内存：假设系统总内存至少是 Go 占用的 4 倍（粗略估算）
	// 如果无法获取真实系统内存，返回保守值
	if totalMB == 0 {
		return 2048, 8192, 2048
	}
	// availMB 估算为 totalMB 的 75%（假设 25% 已被占用）
	availMB = totalMB * 3 / 4
	availCommitMB = availMB
	return
}

// memoryGuard 内存围栏：检测可用内存，返回 GOMEMLIMIT 值和是否需要降级
type memoryGuardResult struct {
	GoMemLimit      string // GOMEMLIMIT 环境变量值
	ShouldDowngrade bool   // 是否需要降低测试数据尺寸
	AvailMB         uint64 // 可用内存 MB
	TotalMB         uint64 // 总内存 MB
}

func memoryGuard() memoryGuardResult {
	availMB, totalMB, availCommitMB := getMemoryInfoMB()
	if availMB == 0 {
		return memoryGuardResult{
			GoMemLimit:      "2GiB",
			ShouldDowngrade: false,
			AvailMB:         0,
			TotalMB:         0,
		}
	}

	limitBaseMB := availMB
	if availCommitMB > 0 && availCommitMB < limitBaseMB {
		limitBaseMB = availCommitMB
	}

	shouldDowngrade := limitBaseMB < 4096

	limitMB := uint64(float64(limitBaseMB) * 0.35)
	if limitMB < 512 {
		limitMB = 512
	}
	if limitMB > 2048 {
		limitMB = 2048
	}

	return memoryGuardResult{
		GoMemLimit:      fmt.Sprintf("%dMiB", limitMB),
		ShouldDowngrade: shouldDowngrade,
		AvailMB:         availMB,
		TotalMB:         totalMB,
	}
}

func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			log.Fatal("找不到项目根目录（go.mod）")
		}
		dir = parent
	}
}

func runBenchmarks(root string, pkgs []packageConfig, benchtime string, perPackageTimeout time.Duration, memGuard memoryGuardResult) ([]benchResult, string, string, error) {
	var allResults []benchResult
	var cpuInfo string
	var goosInfo string

	for i, pkg := range pkgs {
		displayName := fmt.Sprintf("%s [%s]", pkg.Description, pkg.Layer)

		spinner, _ := pterm.DefaultSpinner.
			WithSequence("⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏").
			Start(fmt.Sprintf("[%d/%d] %s 编译中...", i+1, len(pkgs), displayName))

		ctx, cancel := context.WithTimeout(context.Background(), perPackageTimeout)

		args := []string{"test", "-bench=.", "-benchmem", "-json",
			"-benchtime=" + benchtime, "-count=1", "-timeout=300s", "-run=^$"}
		if pkg.BuildTags != "" {
			args = append(args, "-tags", pkg.BuildTags)
		}
		args = append(args, "./"+pkg.Dir)

		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Cancel = func() error {
			killProcessTree(cmd.Process)
			return nil
		}
		cmd.WaitDelay = 5 * time.Second
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GOLANG_LOG=off",
			"GOMEMLIMIT="+memGuard.GoMemLimit,
			"GOGC=50",
		)
		if memGuard.ShouldDowngrade {
			cmd.Env = append(cmd.Env, "ENCV_BENCH_LOW_MEM=1")
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			spinner.Fail(fmt.Sprintf("%s 创建管道失败: %v", displayName, err))
			continue
		}

		if err := cmd.Start(); err != nil {
			cancel()
			spinner.Fail(fmt.Sprintf("%s 启动失败: %v", displayName, err))
			continue
		}

		doneCh := make(chan error, 1)
		go func() {
			doneCh <- cmd.Wait()
		}()

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		var pendingName string
		var pendingPackage string
		var pkgResultCount int
		pkgStart := time.Now()

		for scanner.Scan() {
			line := scanner.Text()
			var evt jsonTestEvent
			if err := json.Unmarshal([]byte(line), &evt); err != nil {
				continue
			}

			if evt.Action != "output" {
				continue
			}

			if strings.HasPrefix(evt.Output, "cpu:") {
				cpuInfo = strings.TrimSpace(strings.TrimPrefix(evt.Output, "cpu:"))
			}
			if strings.HasPrefix(evt.Output, "goos:") {
				goosInfo = strings.TrimSpace(strings.TrimPrefix(evt.Output, "goos:"))
			}

			// 检测到新的子测试开始
			if nameMatches := benchNameLineRe.FindStringSubmatch(evt.Output); nameMatches != nil {
				pendingName = nameMatches[1]
				pendingPackage = evt.Package
				// 提取可读的子测试名
				shortName := pendingName
				if idx := strings.Index(shortName, "/"); idx >= 0 {
					shortName = shortName[idx+1:]
				}
				shortName = strings.TrimPrefix(shortName, "Benchmark")
				elapsed := time.Since(pkgStart).Round(time.Second)
				spinner.UpdateText(fmt.Sprintf("[%d/%d] %s ▸ %s  (%d 项, %s)",
					i+1, len(pkgs), displayName, shortName, pkgResultCount, formatElapsed(elapsed)))
				continue
			}

			if matches := benchLineRe.FindStringSubmatch(evt.Output); matches != nil {
				result := parseBenchMatches(matches, evt.Package, pkg.Layer)
				allResults = append(allResults, result)
				pkgResultCount++
				pendingName = ""
				// 每解析到一条结果就更新 spinner
				shortName := result.Name
				if idx := strings.Index(shortName, "/"); idx >= 0 {
					shortName = shortName[:idx]
				}
				shortName = strings.TrimPrefix(shortName, "Benchmark")
				elapsed := time.Since(pkgStart).Round(time.Second)
				spinner.UpdateText(fmt.Sprintf("[%d/%d] %s ▸ ✓ %s  (%d 项, %s)",
					i+1, len(pkgs), displayName, shortName, pkgResultCount, formatElapsed(elapsed)))
				continue
			}

			if pendingName != "" {
				if valMatches := benchValueLineRe.FindStringSubmatch(evt.Output); valMatches != nil {
					combined := pendingName + " " + strings.TrimSpace(evt.Output)
					if matches := benchLineRe.FindStringSubmatch(combined); matches != nil {
						result := parseBenchMatches(matches, pendingPackage, pkg.Layer)
						allResults = append(allResults, result)
						pkgResultCount++
						shortName := result.Name
						if idx := strings.Index(shortName, "/"); idx >= 0 {
							shortName = shortName[:idx]
						}
						shortName = strings.TrimPrefix(shortName, "Benchmark")
						elapsed := time.Since(pkgStart).Round(time.Second)
						spinner.UpdateText(fmt.Sprintf("[%d/%d] %s ▸ ✓ %s  (%d 项, %s)",
							i+1, len(pkgs), displayName, shortName, pkgResultCount, formatElapsed(elapsed)))
					}
					pendingName = ""
					continue
				}
			}
		}

		select {
		case <-ctx.Done():
			killProcessTree(cmd.Process)
			spinner.Fail(fmt.Sprintf("%s 超时（%v）", displayName, perPackageTimeout))
		case err := <-doneCh:
			if err != nil {
				spinner.Fail(fmt.Sprintf("%s 退出异常", displayName))
			} else {
				elapsed := time.Since(pkgStart).Round(time.Second)
				spinner.Success(fmt.Sprintf("%s: %d 项 (%s)", displayName, pkgResultCount, formatElapsed(elapsed)))
			}
		}

		cancel()
	}

	return allResults, cpuInfo, goosInfo, nil
}

func killProcessTree(p *os.Process) {
	if p == nil {
		return
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.Pid)).Run()
		return
	}
	_ = p.Kill()
}

func parseBenchMatches(matches []string, pkg, layer string) benchResult {
	iterCount, _ := strconv.ParseInt(matches[2], 10, 64)
	nsPerOp, _ := strconv.ParseFloat(matches[3], 64)
	var mbPerSec float64
	if matches[4] != "" {
		mbPerSec, _ = strconv.ParseFloat(matches[4], 64)
	}
	var bytesPerOp int64
	if matches[5] != "" {
		bytesPerOp, _ = strconv.ParseInt(matches[5], 10, 64)
	}
	var allocsPerOp int64
	if matches[6] != "" {
		allocsPerOp, _ = strconv.ParseInt(matches[6], 10, 64)
	}

	fullName := matches[1]
	subName := ""
	if idx := strings.Index(fullName, "/"); idx >= 0 {
		subName = fullName[idx+1:]
	}

	return benchResult{
		Name:        fullName,
		Package:     pkg,
		Layer:       layer,
		IterCount:   iterCount,
		NsPerOp:     nsPerOp,
		MBPerSec:    mbPerSec,
		BytesPerOp:  bytesPerOp,
		AllocsPerOp: allocsPerOp,
		SubName:     subName,
	}
}

func groupByLayer(results []benchResult) []layerInfo {
	layerMap := make(map[string]*layerInfo)
	var layerOrder []string

	for _, r := range results {
		li, ok := layerMap[r.Layer]
		if !ok {
			desc := ""
			for _, p := range packages {
				if p.Layer == r.Layer {
					desc = p.Description
					break
				}
			}
			li = &layerInfo{Name: r.Layer, Description: desc}
			layerMap[r.Layer] = li
			layerOrder = append(layerOrder, r.Layer)
		}
		li.Results = append(li.Results, r)
	}

	var layers []layerInfo
	for _, key := range layerOrder {
		layers = append(layers, *layerMap[key])
	}
	return layers
}

func matchCategory(name string) string {
	base := strings.TrimPrefix(name, "Benchmark")
	// 先匹配完整路径
	for _, cat := range categoryDefs {
		for _, prefix := range cat.BenchPrefixes {
			if strings.Contains(base, prefix) {
				return cat.ID
			}
		}
	}
	// 再匹配顶层前缀
	if idx := strings.Index(base, "/"); idx >= 0 {
		base = base[:idx]
	}
	base = strings.TrimSuffix(base, "_v2")
	for _, cat := range categoryDefs {
		for _, prefix := range cat.BenchPrefixes {
			if strings.HasPrefix(base, prefix) {
				return cat.ID
			}
		}
	}
	return "other"
}

func groupByCategory(results []benchResult) []categoryGroup {
	groups := make(map[string]*categoryGroup)
	var order []string

	for _, r := range results {
		catID := matchCategory(r.Name)
		if _, ok := groups[catID]; !ok {
			var def categoryDef
			for _, d := range categoryDefs {
				if d.ID == catID {
					def = d
					break
				}
			}
			groups[catID] = &categoryGroup{
				ID:          def.ID,
				Name:        def.Name,
				Description: def.Description,
			}
			order = append(order, catID)
		}
		groups[catID].Results = append(groups[catID].Results, r)
	}

	var result []categoryGroup
	for _, id := range order {
		g := groups[id]
		var totalScore float64
		for _, r := range g.Results {
			desc := getBenchDesc(r.Name)
			grade := getGrade(r, desc)
			totalScore += calcItemScore(r, desc)
			switch grade {
			case "excellent":
				g.ExcelCount++
			case "good":
				g.GoodCount++
			case "warn":
				g.WarnCount++
			}
		}
		if len(g.Results) > 0 {
			g.Score = totalScore / float64(len(g.Results))
		}
		result = append(result, *g)
	}
	return result
}

func calcItemScore(r benchResult, desc benchDesc) float64 {
	if desc.Unit == "吞吐量" && r.MBPerSec > 0 && desc.ExcelMB > 0 {
		return math.Min(100, (r.MBPerSec/desc.ExcelMB)*100)
	}
	if desc.Unit == "延迟" && r.NsPerOp > 0 && desc.ExcelNs > 0 {
		return math.Min(100, (desc.ExcelNs/r.NsPerOp)*100)
	}
	return 50
}

func calcOverallScore(results []benchResult) float64 {
	if len(results) == 0 {
		return 0
	}
	var total float64
	for _, r := range results {
		desc := getBenchDesc(r.Name)
		total += calcItemScore(r, desc)
	}
	return total / float64(len(results))
}

func generateInsights(results []benchResult, historyResults []benchResult) []insightItem {
	var insights []insightItem

	var bestBench *benchResult
	for i := range results {
		if results[i].MBPerSec > 0 {
			if bestBench == nil || results[i].MBPerSec > bestBench.MBPerSec {
				bestBench = &results[i]
			}
		}
	}
	if bestBench != nil {
		desc := getBenchDesc(bestBench.Name)
		insights = append(insights, insightItem{
			Type:    "excellent",
			Message: fmt.Sprintf("%s 吞吐量达 %.0f MB/s，远超优秀线（%d MB/s）", desc.Category, bestBench.MBPerSec, int(desc.ExcelMB)),
		})
	}

	for _, r := range results {
		desc := getBenchDesc(r.Name)
		grade := getGrade(r, desc)
		if grade == "warn" {
			msg := fmt.Sprintf("%s 性能偏慢", desc.Category)
			if desc.Unit == "吞吐量" && desc.GoodMB > 0 {
				msg += fmt.Sprintf("（%.0f MB/s < 及格线 %d MB/s）", r.MBPerSec, int(desc.GoodMB))
			} else if desc.Unit == "延迟" && desc.GoodNs > 0 {
				msg += fmt.Sprintf("（%s > 及格线 %s）", formatDuration(r.NsPerOp), formatDuration(desc.GoodNs))
			}
			if desc.Note != "" {
				msg += "，" + desc.Note
			}
			insights = append(insights, insightItem{Type: "warn", Message: msg})
		}
	}

	if len(historyResults) > 0 {
		prevMap := make(map[string]benchResult)
		for _, r := range historyResults {
			prevMap[r.Name] = r
		}
		var biggestGain string
		var biggestGainPct float64
		for _, r := range results {
			if prev, ok := prevMap[r.Name]; ok {
				desc := getBenchDesc(r.Name)
				if desc.Unit == "吞吐量" && r.MBPerSec > 0 && prev.MBPerSec > 0 {
					pct := (r.MBPerSec - prev.MBPerSec) / prev.MBPerSec * 100
					if pct > biggestGainPct {
						biggestGainPct = pct
						biggestGain = fmt.Sprintf("%s 吞吐量提升 %.1f%%（%.0f → %.0f MB/s）", desc.Category, pct, prev.MBPerSec, r.MBPerSec)
					}
				}
			}
		}
		if biggestGain != "" {
			insights = append(insights, insightItem{Type: "compare", Message: biggestGain})
		}
	}

	return insights
}

func formatDuration(ns float64) string {
	switch {
	case ns < 1000:
		return fmt.Sprintf("%.1f ns", ns)
	case ns < 1_000_000:
		return fmt.Sprintf("%.1f μs", ns/1000)
	case ns < 1_000_000_000:
		return fmt.Sprintf("%.2f ms", ns/1_000_000)
	default:
		return fmt.Sprintf("%.2f s", ns/1_000_000_000)
	}
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func throughputClass(mbps float64) string {
	switch {
	case mbps >= 5000:
		return "ultra"
	case mbps >= 1000:
		return "high"
	case mbps >= 100:
		return "medium"
	case mbps > 0:
		return "low"
	default:
		return "none"
	}
}

func latencyClass(ns float64) string {
	switch {
	case ns < 100:
		return "ultra"
	case ns < 1000:
		return "fast"
	case ns < 100_000:
		return "medium"
	case ns < 1_000_000:
		return "slow"
	default:
		return "very-slow"
	}
}

// 历史缓存读写
func benchDir(root string) string {
	return filepath.Join(root, "tests", "bench")
}

func ensureBenchDir(root string) error {
	return os.MkdirAll(benchDir(root), 0755)
}

func historyPath(root string) string {
	return filepath.Join(benchDir(root), "bench-history.json")
}

func loadHistory(root string) (*historyFile, error) {
	data, err := os.ReadFile(historyPath(root))
	if err != nil {
		return nil, err
	}
	var h historyFile
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func saveHistory(root string, h *historyFile) error {
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(historyPath(root), data, 0644)
}

var funcMap = template.FuncMap{
	"fmtDuration":     formatDuration,
	"fmtBytes":        formatBytes,
	"throughputClass": throughputClass,
	"latencyClass":    latencyClass,
	"printf":          fmt.Sprintf,
	"hasThroughput":   func(r benchResult) bool { return r.MBPerSec > 0 },
	"jsq": func(s string) template.JS {
		return template.JS(fmt.Sprintf("%q", s))
	},
	"jsn": func(f float64, format string) template.JS {
		return template.JS(fmt.Sprintf(format, f))
	},
	"shortPkg": func(pkg string) string {
		parts := strings.Split(pkg, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return pkg
	},
	"layerColor": func(layer string) string {
		colors := map[string]string{
			"L1":    "#00d4aa",
			"L2":    "#4ecdc4",
			"L3":    "#45b7d1",
			"L4/L5": "#f7b731",
		}
		if c, ok := colors[layer]; ok {
			return c
		}
		return "#888"
	},
	"layerOrder": func(layer string) int {
		orders := map[string]int{"L1": 1, "L2": 2, "L3": 3, "L4/L5": 4}
		if o, ok := orders[layer]; ok {
			return o
		}
		return 5
	},
	"barWidth": func(value, max float64) float64 {
		if max <= 0 {
			return 0
		}
		pct := value / max * 100
		if pct > 100 {
			pct = 100
		}
		return pct
	},
	"sortResults": func(results []benchResult) []benchResult {
		sorted := make([]benchResult, len(results))
		copy(sorted, results)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Name < sorted[j].Name
		})
		return sorted
	},
	"maxThroughput": func(results []benchResult) float64 {
		var max float64
		for _, r := range results {
			if r.MBPerSec > max {
				max = r.MBPerSec
			}
		}
		return max
	},
	"getDesc":  getBenchDesc,
	"getGrade": getGrade,
	"gradeLabel": func(g string) string {
		switch g {
		case "excellent":
			return "优秀"
		case "good":
			return "及格"
		case "warn":
			return "偏慢"
		default:
			return ""
		}
	},
	"gradeIcon": func(g string) string {
		switch g {
		case "excellent":
			return "★"
		case "good":
			return "✓"
		case "warn":
			return "△"
		default:
			return ""
		}
	},
	"pctChange": func(cur, prev float64) float64 {
		if prev == 0 {
			return 0
		}
		return (cur - prev) / prev * 100
	},
	"abs": math.Abs,
	"mul": func(args ...float64) float64 {
		result := 1.0
		for _, a := range args {
			result *= a
		}
		return result
	},
	"scoreColor": func(score float64) string {
		switch {
		case score >= 80:
			return "#22c55e"
		case score >= 50:
			return "#00d4aa"
		case score >= 30:
			return "#f7b731"
		default:
			return "#ff6b6b"
		}
	},
	"scoreLabel": func(score float64) string {
		switch {
		case score >= 80:
			return "优秀"
		case score >= 50:
			return "良好"
		case score >= 30:
			return "一般"
		default:
			return "偏慢"
		}
	},
	"scoreRingOffset": func(score float64) float64 {
		circumference := 2 * math.Pi * 52
		return circumference * (1 - score/100)
	},
	"categoryColor": func(id string) string {
		colors := map[string]string{
			"key-derivation": "#a78bfa",
			"aes-ctr":        "#00d4aa",
			"container-io":   "#45b7d1",
			"envelope":       "#e879f9",
			"manifest":       "#fb923c",
			"fragment":       "#f7b731",
			"physical":       "#38bdf8",
			"decrypt-reader": "#ff6b6b",
			"concurrency":    "#34d399",
		}
		if c, ok := colors[id]; ok {
			return c
		}
		return "#888"
	},
	"insightIcon": func(t string) string {
		switch t {
		case "excellent":
			return "★"
		case "warn":
			return "△"
		case "compare":
			return "↗"
		default:
			return "●"
		}
	},
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>encv-go 基准测试报告</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.7/dist/chart.umd.min.js"></script>
<style>
:root {
  --bg-deep: #080a0e;
  --bg-base: #0c0e14;
  --bg-surface: #12151e;
  --bg-elevated: #1a1e2c;
  --bg-hover: #222738;
  --border: #262b3d;
  --border-subtle: #1a1f30;
  --text-primary: #e8eaf0;
  --text-secondary: #9ca3b8;
  --text-muted: #5c6378;
  --accent: #00d4aa;
  --accent-dim: rgba(0,212,170,0.12);
  --accent-glow: rgba(0,212,170,0.25);
  --red: #ff6b6b;
  --red-dim: rgba(255,107,107,0.12);
  --orange: #f7b731;
  --orange-dim: rgba(247,183,49,0.12);
  --blue: #45b7d1;
  --blue-dim: rgba(69,183,209,0.12);
  --purple: #a78bfa;
  --purple-dim: rgba(167,139,250,0.12);
  --green: #22c55e;
  --green-dim: rgba(34,197,94,0.12);
  --font-sans: system-ui, -apple-system, sans-serif;
  --font-mono: 'Consolas', 'Courier New', monospace;
  --radius-sm: 6px;
  --radius-md: 10px;
  --radius-lg: 14px;
  --radius-xl: 20px;
}
*{margin:0;padding:0;box-sizing:border-box}
body{background:var(--bg-deep);color:var(--text-primary);font-family:var(--font-sans);line-height:1.6;-webkit-font-smoothing:antialiased}
.container{max-width:1200px;margin:0 auto;padding:0 28px}

.report-header{position:relative;padding:48px 0 36px;border-bottom:1px solid var(--border-subtle);overflow:hidden}
.report-header::before{content:'';position:absolute;top:-180px;right:-180px;width:500px;height:500px;background:radial-gradient(circle,var(--accent-glow) 0%,transparent 70%);opacity:0.25;pointer-events:none}
.report-header .badge{display:inline-flex;align-items:center;gap:6px;padding:5px 12px;background:var(--accent-dim);border:1px solid rgba(0,212,170,0.2);border-radius:100px;font-size:11px;font-weight:600;color:var(--accent);letter-spacing:0.5px;text-transform:uppercase;margin-bottom:14px}
.report-header h1{font-size:clamp(24px,3.5vw,38px);font-weight:800;letter-spacing:-0.8px;line-height:1.15;margin-bottom:8px}
.report-header h1 span{color:var(--accent)}
.report-header .subtitle{font-size:14px;color:var(--text-secondary);max-width:600px}
.meta-row{display:flex;flex-wrap:wrap;gap:8px;margin-top:20px}
.meta-chip{display:inline-flex;align-items:center;gap:5px;padding:4px 10px;background:var(--bg-surface);border:1px solid var(--border-subtle);border-radius:6px;font-size:11px;font-family:var(--font-mono);color:var(--text-secondary)}
.meta-chip .label{color:var(--text-muted);font-family:var(--font-sans);font-weight:500}

.dashboard{display:grid;grid-template-columns:220px 1fr;gap:24px;padding:32px 0 24px;align-items:start}
@media(max-width:768px){.dashboard{grid-template-columns:1fr}}

.score-panel{display:flex;flex-direction:column;align-items:center;gap:12px}
.score-ring-wrap{position:relative;width:180px;height:180px}
.score-ring-wrap svg{width:100%;height:100%;transform:rotate(-90deg)}
.score-ring-wrap .ring-bg{fill:none;stroke:var(--bg-elevated);stroke-width:10}
.score-ring-wrap .ring-fg{fill:none;stroke-width:10;stroke-linecap:round;transition:stroke-dashoffset 1.2s cubic-bezier(0.16,1,0.3,1)}
.score-center{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center}
.score-number{font-size:42px;font-weight:800;line-height:1;font-family:var(--font-mono)}
.score-suffix{font-size:13px;color:var(--text-muted);margin-top:2px}
.score-verdict{font-size:14px;font-weight:700;padding:4px 14px;border-radius:100px}

.radar-panel{background:var(--bg-surface);border:1px solid var(--border-subtle);border-radius:var(--radius-lg);padding:24px;position:relative;overflow:hidden}
.radar-panel::before{content:'';position:absolute;top:0;left:0;right:0;height:2px;background:linear-gradient(90deg,var(--accent),var(--blue),var(--purple))}
.radar-panel h3{font-size:14px;font-weight:700;margin-bottom:16px;color:var(--text-secondary)}
.radar-canvas-wrap{max-width:420px;margin:0 auto}

.insights-section{padding:0 0 28px}
.insights-section h3{font-size:14px;font-weight:700;color:var(--text-secondary);margin-bottom:12px}
.insights-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:10px}
.insight-card{display:flex;align-items:flex-start;gap:10px;padding:12px 16px;background:var(--bg-surface);border:1px solid var(--border-subtle);border-radius:var(--radius-md);font-size:13px;line-height:1.5;transition:border-color 0.2s}
.insight-card:hover{border-color:var(--border)}
.insight-card .icon{flex-shrink:0;width:24px;height:24px;border-radius:6px;display:flex;align-items:center;justify-content:center;font-size:12px;font-weight:800}
.insight-card.excellent .icon{background:var(--green-dim);color:var(--green)}
.insight-card.warn .icon{background:var(--orange-dim);color:var(--orange)}
.insight-card.compare .icon{background:var(--blue-dim);color:var(--blue)}
.insight-card .msg{color:var(--text-secondary)}
.insight-card.excellent .msg strong{color:var(--green)}
.insight-card.warn .msg strong{color:var(--orange)}
.insight-card.compare .msg strong{color:var(--blue)}

.category-section{padding:28px 0}
.category-header{display:flex;align-items:flex-start;gap:14px;margin-bottom:18px}
.category-accent{width:4px;min-height:48px;border-radius:2px;flex-shrink:0;margin-top:2px}
.category-info{flex:1}
.category-info h2{font-size:20px;font-weight:700;letter-spacing:-0.3px;margin-bottom:4px}
.category-info .desc{font-size:13px;color:var(--text-secondary);line-height:1.6;margin-bottom:10px}
.category-stats{display:flex;gap:8px;flex-wrap:wrap}
.cat-stat{display:inline-flex;align-items:center;gap:4px;padding:3px 10px;border-radius:100px;font-size:11px;font-weight:600}
.cat-stat.excel{background:var(--green-dim);color:var(--green)}
.cat-stat.good{background:var(--accent-dim);color:var(--accent)}
.cat-stat.warn{background:var(--orange-dim);color:var(--orange)}
.category-score{display:flex;flex-direction:column;align-items:center;gap:2px;flex-shrink:0}
.category-score .val{font-size:28px;font-weight:800;font-family:var(--font-mono);line-height:1}
.category-score .lbl{font-size:10px;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.5px}

.bench-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(340px,1fr));gap:10px}
.bench-card{background:var(--bg-surface);border:1px solid var(--border-subtle);border-radius:var(--radius-md);padding:14px 16px;transition:border-color 0.2s,background 0.2s}
.bench-card:hover{border-color:var(--border);background:var(--bg-elevated)}
.bench-card .bench-top{display:flex;align-items:center;justify-content:space-between;gap:8px;margin-bottom:6px}
.bench-card .bench-name{font-family:var(--font-mono);font-size:11.5px;font-weight:600;color:var(--text-primary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.bench-card .bench-desc{font-size:11px;color:var(--text-muted);line-height:1.4;margin-bottom:10px}
.bench-card .bench-metrics{display:flex;flex-wrap:wrap;gap:8px;align-items:center}
.metric{display:flex;align-items:center;gap:5px}
.metric .m-label{font-size:10px;color:var(--text-muted);font-weight:500}
.metric .m-value{font-family:var(--font-mono);font-size:12px;font-weight:600}
.metric .m-value.lat{color:var(--blue)}
.metric .m-value.tput{color:var(--accent)}
.metric .m-value.mem{color:var(--text-secondary)}

.grade-pill{display:inline-flex;align-items:center;gap:3px;padding:2px 8px;border-radius:4px;font-size:10px;font-weight:700;letter-spacing:0.3px;flex-shrink:0}
.grade-pill.excellent{background:var(--green-dim);color:var(--green)}
.grade-pill.good{background:var(--accent-dim);color:var(--accent)}
.grade-pill.warn{background:var(--orange-dim);color:var(--orange)}

.tput-bar-wrap{flex:1;min-width:60px;max-width:120px;height:4px;background:var(--bg-deep);border-radius:2px;overflow:hidden}
.tput-bar{height:100%;border-radius:2px;transition:width 0.8s cubic-bezier(0.16,1,0.3,1)}
.tput-bar.ultra{background:linear-gradient(90deg,#00d4aa,#22c55e)}
.tput-bar.high{background:linear-gradient(90deg,#45b7d1,#00d4aa)}
.tput-bar.medium{background:linear-gradient(90deg,#f7b731,#ff6b6b)}
.tput-bar.low{background:#ff6b6b}

.diff-tag{font-family:var(--font-mono);font-size:10px;font-weight:600;padding:2px 6px;border-radius:3px}
.diff-tag.better{background:var(--green-dim);color:var(--green)}
.diff-tag.worse{background:var(--red-dim);color:var(--red)}
.diff-tag.same{color:var(--text-muted)}

.ref-section{padding:20px 0 28px}
.ref-section h3{font-size:14px;font-weight:700;color:var(--text-secondary);margin-bottom:10px}
.ref-section .ref-note{font-size:11px;color:var(--text-muted);margin-bottom:12px}
.ref-table{width:100%;border-collapse:separate;border-spacing:0;background:var(--bg-surface);border:1px solid var(--border-subtle);border-radius:var(--radius-md);overflow:hidden}
.ref-table th{background:var(--bg-elevated);padding:9px 12px;font-size:10px;font-weight:600;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.7px;text-align:left;border-bottom:1px solid var(--border)}
.ref-table td{padding:7px 12px;font-size:11.5px;border-bottom:1px solid var(--border-subtle)}
.ref-table tr:last-child td{border-bottom:none}

.report-footer{padding:24px 0;border-top:1px solid var(--border-subtle);text-align:center;color:var(--text-muted);font-size:11px}

@keyframes fadeInUp{from{opacity:0;transform:translateY(14px)}to{opacity:1;transform:translateY(0)}}
.anim{animation:fadeInUp 0.45s cubic-bezier(0.16,1,0.3,1) both}
@media(max-width:768px){.container{padding:0 14px}.bench-grid{grid-template-columns:1fr}.dashboard{grid-template-columns:1fr}}
</style>
</head>
<body>
<div class="container">

<header class="report-header anim">
  <div class="badge">Benchmark Report</div>
  <h1>encv-go <span>性能基准</span>测试报告</h1>
  <p class="subtitle">视频加密 / 流式预览 / 解密全链路性能分析</p>
  <div class="meta-row">
    <span class="meta-chip"><span class="label">时间</span>{{.Timestamp}}</span>
    <span class="meta-chip"><span class="label">平台</span>{{.OS}}/{{.Arch}}</span>
    <span class="meta-chip"><span class="label">CPU</span>{{.CPU}}</span>
    <span class="meta-chip"><span class="label">Go</span>{{.GoVersion}}</span>
    <span class="meta-chip"><span class="label">测试项</span>{{.PassCount}}</span>
    {{if .Calibration.AESThroughput}}<span class="meta-chip"><span class="label">校准</span>{{printf "%.0f" .Calibration.AESThroughput}} MB/s ({{printf "%.2f" .Calibration.CPUScore}}x)</span>{{end}}
    {{if .HasHistory}}<span class="meta-chip"><span class="label">上次</span>{{.HistoryTime}}</span>{{end}}
  </div>
</header>

<section class="dashboard anim" style="animation-delay:0.05s">
  <div class="score-panel">
    <div class="score-ring-wrap">
      <svg viewBox="0 0 120 120">
        <circle class="ring-bg" cx="60" cy="60" r="52"/>
        <circle class="ring-fg" cx="60" cy="60" r="52"
          stroke="{{scoreColor .OverallScore}}"
          stroke-dasharray="{{printf "%.2f" (mul 2 3.14159265 52)}}"
          stroke-dashoffset="{{printf "%.2f" (scoreRingOffset .OverallScore)}}"/>
      </svg>
      <div class="score-center">
        <span class="score-number" style="color:{{scoreColor .OverallScore}}">{{printf "%.0f" .OverallScore}}</span>
        <span class="score-suffix">/ 100</span>
      </div>
    </div>
    <span class="score-verdict" style="background:{{scoreColor .OverallScore}}22;color:{{scoreColor .OverallScore}}">{{scoreLabel .OverallScore}}</span>
  </div>
  <div class="radar-panel">
    <h3>分类性能雷达</h3>
    <div class="radar-canvas-wrap">
      <canvas id="radarChart"></canvas>
    </div>
  </div>
</section>

{{if .Insights}}
<section class="insights-section anim" style="animation-delay:0.1s">
  <h3>关键发现</h3>
  <div class="insights-grid">
    {{range .Insights}}
    <div class="insight-card {{.Type}}">
      <div class="icon">{{insightIcon .Type}}</div>
      <div class="msg">{{.Message}}</div>
    </div>
    {{end}}
  </div>
</section>
{{end}}

{{range .CategoryGroups}}
<section class="category-section anim" style="animation-delay:0.12s">
  <div class="category-header">
    <div class="category-accent" style="background:{{categoryColor .ID}}"></div>
    <div class="category-info">
      <h2>{{.Name}}</h2>
      <p class="desc">{{.Description}}</p>
      <div class="category-stats">
        {{if .ExcelCount}}<span class="cat-stat excel">★ 优秀 {{.ExcelCount}}</span>{{end}}
        {{if .GoodCount}}<span class="cat-stat good">✓ 及格 {{.GoodCount}}</span>{{end}}
        {{if .WarnCount}}<span class="cat-stat warn">△ 偏慢 {{.WarnCount}}</span>{{end}}
      </div>
    </div>
    <div class="category-score">
      <span class="val" style="color:{{scoreColor .Score}}">{{printf "%.0f" .Score}}</span>
      <span class="lbl">score</span>
    </div>
  </div>
  <div class="bench-grid">
  {{range sortResults .Results}}
    {{$r := .}}
    {{with $desc := getDesc $r.Name}}
    {{with $grade := getGrade $r $desc}}
    <div class="bench-card">
      <div class="bench-top">
        <span class="bench-name" title="{{$desc.Description}}">{{$r.Name}}</span>
        {{if $grade}}<span class="grade-pill {{$grade}}">{{gradeIcon $grade}} {{gradeLabel $grade}}</span>{{end}}
      </div>
      <div class="bench-desc">{{$desc.Description}}</div>
      <div class="bench-metrics">
        <div class="metric">
          <span class="m-label">延迟</span>
          <span class="m-value lat">{{fmtDuration $r.NsPerOp}}</span>
        </div>
        {{if hasThroughput $r}}
        <div class="metric" style="flex:1;min-width:140px">
          <span class="m-label">吞吐</span>
          <div class="tput-bar-wrap">
            <div class="tput-bar {{throughputClass $r.MBPerSec}}" data-width="{{printf "%.1f" (barWidth $r.MBPerSec (maxThroughput $.Results))}}"></div>
          </div>
          <span class="m-value tput">{{printf "%.0f" $r.MBPerSec}}</span>
        </div>
        {{end}}
        <div class="metric">
          <span class="m-label">内存</span>
          <span class="m-value mem">{{fmtBytes $r.BytesPerOp}}</span>
        </div>
        {{if $.HasHistory}}<span class="diff-cell" data-bench="{{$r.Name}}" style="padding:0"></span>{{end}}
      </div>
    </div>
    {{end}}
    {{end}}
  {{end}}
  </div>
</section>
{{end}}

<section class="ref-section anim" style="animation-delay:0.15s">
  <h3>评级参考标准</h3>
  <p class="ref-note">基于 AES-256-CTR 在现代 x86 CPU（AES-NI）上的典型表现，实际性能受 CPU 型号、内存带宽、磁盘 I/O 影响</p>
  <table class="ref-table">
    <thead><tr>
      <th>类别</th><th>指标</th><th style="color:var(--green)">★ 优秀</th><th style="color:var(--accent)">✓ 及格</th><th style="color:var(--orange)">△ 偏慢</th><th>说明</th>
    </tr></thead>
    <tbody id="refBody"></tbody>
  </table>
</section>

<footer class="report-footer">
  由 encv-go bench-report 自动生成 · {{.Timestamp}}{{if .HasHistory}} · 历史对比: {{.HistoryTime}}{{end}}
</footer>
</div>

<script>
(function(){
  var allResults = [{{range .Results}}{name:{{.Name | jsq}},layer:{{.Layer | jsq}},ns:{{jsn .NsPerOp "%.0f"}},mbps:{{jsn .MBPerSec "%.1f"}},bytes:{{.BytesPerOp}},allocs:{{.AllocsPerOp}}},{{end}}];
  var hasHistory = {{.HasHistory}};
  var prevMap = {};
  if (hasHistory) {
    var prevResults = [{{range .HistoryResults}}{name:{{.Name | jsq}},ns:{{jsn .NsPerOp "%.0f"}},mbps:{{jsn .MBPerSec "%.1f"}},bytes:{{.BytesPerOp}},allocs:{{.AllocsPerOp}}},{{end}}];
    prevResults.forEach(function(r){prevMap[r.name]=r});
  }

  var descMap = {};
  {{range .Results}}{{$r := .}}{{with $d := getDesc $r.Name}}descMap[{{$r.Name | jsq}}]={cat:{{$d.Category | jsq}},desc:{{$d.Description | jsq}},unit:{{$d.Unit | jsq}},goodMB:{{$d.GoodMB}},excelMB:{{$d.ExcelMB}},goodNs:{{$d.GoodNs}},excelNs:{{$d.ExcelNs}},note:{{$d.Note | jsq}}};{{end}}{{end}}

  var catDefs = [
    {id:'key-derivation',name:'密钥派生',color:'#a78bfa'},
    {id:'aes-ctr',name:'AES-CTR 加解密',color:'#00d4aa'},
    {id:'container-io',name:'容器 Block I/O',color:'#45b7d1'},
    {id:'envelope',name:'信封与检测',color:'#e879f9'},
    {id:'manifest',name:'清单序列化',color:'#fb923c'},
    {id:'fragment',name:'分片管理',color:'#f7b731'},
    {id:'physical',name:'物理打包/解包',color:'#38bdf8'},
    {id:'decrypt-reader',name:'解密读取器',color:'#ff6b6b'},
    {id:'concurrency',name:'并发与 Seek 矩阵',color:'#34d399'}
  ];

  // ---- 雷达图 ----
  var catScores = {};
  allResults.forEach(function(r){
    var d = descMap[r.name]; if(!d) return;
    var cat = matchCat(r.name);
    if(!catScores[cat]) catScores[cat] = [];
    catScores[cat].push(calcScore(r,d));
  });
  var radarLabels = [], radarData = [], radarColors = [];
  catDefs.forEach(function(c){
    var arr = catScores[c.id] || [50];
    var avg = arr.reduce(function(a,b){return a+b},0)/arr.length;
    radarLabels.push(c.name);
    radarData.push(Math.round(avg));
    radarColors.push(c.color);
  });
  new Chart(document.getElementById('radarChart'),{
    type:'radar',
    data:{
      labels:radarLabels,
      datasets:[{
        data:radarData,
        backgroundColor:'rgba(0,212,170,0.08)',
        borderColor:'#00d4aa',
        borderWidth:2,
        pointBackgroundColor:radarColors,
        pointBorderColor:radarColors,
        pointRadius:5,
        pointHoverRadius:7
      }]
    },
    options:{
      responsive:true,
      maintainAspectRatio:true,
      scales:{
        r:{
          beginAtZero:true,
          max:100,
          ticks:{stepSize:25,color:'#5c6378',backdropColor:'transparent',font:{size:9}},
          grid:{color:'rgba(255,255,255,0.06)'},
          angleLines:{color:'rgba(255,255,255,0.06)'},
          pointLabels:{color:'#9ca3b8',font:{size:11,weight:'600'}}
        }
      },
      plugins:{legend:{display:false}}
    }
  });

  function matchCat(name){
    var base=name.replace(/^Benchmark/,'');
    if(base.indexOf('/')>=0) base=base.substring(0,base.indexOf('/'));
    base=base.replace(/_v2$/,'');
    var map=[
      {id:'key-derivation',pfx:['GenerateKey','GenerateSalt','GenerateIV']},
      {id:'aes-ctr',pfx:['EncryptStream','DecryptStream','EncryptBytes','DecryptBytes','DeriveCTRIV']},
      {id:'container-io',pfx:['EncryptSystemPayload','DecryptSystemPayload','WriteBlock','ReadBlockHeader','ReadBlockData','EncryptToTempFile']},
      {id:'envelope',pfx:['ParseEnvelope','ReadEnvelope','IsEncvContainer','DetectContainer','DetectIndexKind']},
      {id:'manifest',pfx:['SerializeManifest','DeserializeManifest','EncryptManifest','DecryptManifest']},
      {id:'fragment',pfx:['CalculateFragmentSize','CreateLogicalFragments','ValidateGlobalStartOffsets']},
      {id:'physical',pfx:['SinglePhysical','FileChunkerPhysical']},
      {id:'decrypt-reader',pfx:['DecryptReaderFactory','SequentialDecryptReader','VirtualSeekable','SequentialSeekable','BulkDecryptor']},
      {id:'concurrency',pfx:['ConcurrentDecryptReader','ConcurrentSeekRead','SeekMatrix']}
    ];
    for(var i=0;i<map.length;i++){
      for(var j=0;j<map[i].pfx.length;j++){
        if(base.indexOf(map[i].pfx[j])===0) return map[i].id;
      }
    }
    return 'other';
  }

  function calcScore(r,d){
    if(d.unit==='吞吐量'&&r.mbps>0&&d.excelMB>0) return Math.min(100,(r.mbps/d.excelMB)*100);
    if(d.unit==='延迟'&&r.ns>0&&d.excelNs>0) return Math.min(100,(d.excelNs/r.ns)*100);
    return 50;
  }

  // ---- 评级参考表 ----
  var refBody = document.getElementById('refBody');
  var seen = {};
  allResults.forEach(function(r){
    var d = descMap[r.name];
    if(!d || seen[d.cat]) return;
    seen[d.cat] = true;
    var tr = document.createElement('tr');
    if(d.unit==='吞吐量'){
      tr.innerHTML='<td>'+d.cat+'</td><td>吞吐量</td>'+
        '<td style="color:var(--green)">≥ '+d.excelMB+' MB/s</td>'+
        '<td style="color:var(--accent)">≥ '+d.goodMB+' MB/s</td>'+
        '<td style="color:var(--orange)">&lt; '+d.goodMB+' MB/s</td>'+
        '<td style="color:var(--text-muted)">'+(d.note||'')+'</td>';
    } else {
      tr.innerHTML='<td>'+d.cat+'</td><td>延迟</td>'+
        '<td style="color:var(--green)">≤ '+fmtNs(d.excelNs)+'</td>'+
        '<td style="color:var(--accent)">≤ '+fmtNs(d.goodNs)+'</td>'+
        '<td style="color:var(--orange)">&gt; '+fmtNs(d.goodNs)+'</td>'+
        '<td style="color:var(--text-muted)">'+(d.note||'')+'</td>';
    }
    refBody.appendChild(tr);
  });

  // ---- 历史对比 ----
  if(hasHistory){
    document.querySelectorAll('[data-bench]').forEach(function(el){
      var name = el.dataset.bench;
      var prev = prevMap[name];
      if(!prev){el.innerHTML='<span class="diff-tag same">新增</span>';return;}
      var cur = null;
      allResults.forEach(function(r){if(r.name===name) cur=r;});
      if(!cur) return;
      var d = descMap[name];
      var parts = [];
      if(d && d.unit==='吞吐量' && cur.mbps>0 && prev.mbps>0){
        var pct = ((cur.mbps-prev.mbps)/prev.mbps*100);
        parts.push(makeDiff(pct,'MB/s'));
      }
      if(cur.ns>0 && prev.ns>0){
        var nsPct = ((cur.ns-prev.ns)/prev.ns*100);
        parts.push(makeDiff(-nsPct,'延迟'));
      }
      el.innerHTML = parts.join(' ') || '<span class="diff-tag same">持平</span>';
    });
  }

  function makeDiff(pct,label){
    if(Math.abs(pct)<1) return '<span class="diff-tag same">'+label+' 持平</span>';
    if(pct>0) return '<span class="diff-tag better">▲'+label+'+'+pct.toFixed(1)+'%</span>';
    return '<span class="diff-tag worse">▼'+label+' '+pct.toFixed(1)+'%</span>';
  }

  function fmtNs(ns){
    if(ns<1000) return ns.toFixed(1)+' ns';
    if(ns<1e6) return (ns/1000).toFixed(1)+' μs';
    if(ns<1e9) return (ns/1e6).toFixed(2)+' ms';
    return (ns/1e9).toFixed(2)+' s';
  }

  // ---- 吞吐量条动画 ----
  setTimeout(function(){
    document.querySelectorAll('.tput-bar').forEach(function(bar){
      var w = bar.dataset.width;
      bar.style.width = '0%';
      requestAnimationFrame(function(){
        requestAnimationFrame(function(){bar.style.width = w+'%';});
      });
    });
  },200);
})();
</script>
</body>
</html>`

func generateReport(data *reportData, outputPath string) error {
	tmpl, err := template.New("report").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("解析模板失败: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("渲染模板失败: %w", err)
	}

	return nil
}

// runCalibration 运行硬件校准基准，返回校准结果
// 使用 performance.RunCalibration()（跨平台，纯 Go AES-CTR 测试）
func runCalibration(root string) calibrationResult {
	spinner, _ := pterm.DefaultSpinner.
		WithSequence("⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏").
		Start("硬件校准中...")

	cal := performance.RunCalibration()

	if cal.AESThroughput <= 0 {
		spinner.Warning("硬件校准失败，使用默认评级标准")
		return calibrationResult{CPUScore: 1.0, AESThroughput: 0, CPULabel: "unknown"}
	}

	spinner.Success(fmt.Sprintf("AES-CTR %.0f MB/s | CPU 分数 %.2f (%s)", cal.AESThroughput, cal.CPUScore, cal.CPULabel))

	return calibrationResult{
		CPUScore:      cal.CPUScore,
		AESThroughput: cal.AESThroughput,
		CPULabel:      cal.CPULabel,
	}
}

// applyCalibration 根据硬件校准结果动态调整评级标准
func applyCalibration(descs map[string]benchDesc, cal calibrationResult) map[string]benchDesc {
	if cal.CPUScore <= 0 || cal.CPUScore == 1.0 {
		return descs
	}

	adjusted := make(map[string]benchDesc, len(descs))
	for k, d := range descs {
		ad := d
		// 吞吐量标准：CPU 越快，及格线越高
		if d.Unit == "吞吐量" && d.GoodMB > 0 {
			ad.GoodMB = d.GoodMB * cal.CPUScore
			ad.ExcelMB = d.ExcelMB * cal.CPUScore
		}
		// 延迟标准：CPU 越快，及格线越低
		if d.Unit == "延迟" && d.GoodNs > 0 {
			ad.GoodNs = d.GoodNs / cal.CPUScore
			ad.ExcelNs = d.ExcelNs / cal.CPUScore
		}
		adjusted[k] = ad
	}
	return adjusted
}

func main() {
	benchtime := flag.String("benchtime", "2s", "基准测试运行时间 (如 2s, 5s, 1x)")
	output := flag.String("o", "", "输出 HTML 文件路径（默认 tests/bench/bench-report.html）")
	openBrowser := flag.Bool("open", true, "生成后自动在浏览器中打开")
	skipL4 := flag.Bool("skip-integration", false, "跳过 L4/L5 集成测试（需要 FFmpeg 和视频文件）")
	noSave := flag.Bool("no-save", false, "不保存本次结果到历史缓存")
	pkgTimeout := flag.Duration("pkg-timeout", 10*time.Minute, "单个测试包超时时间（防止卡死）")
	storePath := flag.String("store", "", "SQLite 数据库路径（可选，把结果写入 performance_metrics 表）")
	flag.Parse()

	pterm.DefaultHeader.WithFullWidth().Println("encv-go 基准测试报告生成器")

	root := findProjectRoot()
	pterm.Info.Printf("项目根目录: %s\n", root)

	// 内存围栏检测
	memGuard := memoryGuard()
	if memGuard.AvailMB > 0 {
		memLabel := pterm.Green(fmt.Sprintf("%d MB", memGuard.AvailMB))
		if memGuard.ShouldDowngrade {
			memLabel = pterm.Yellow(fmt.Sprintf("%d MB ⚠ 低内存", memGuard.AvailMB))
		}
		pterm.Info.Printf("内存: 可用 %s / 总计 %d MB | GOMEMLIMIT=%s\n",
			memLabel, memGuard.TotalMB, memGuard.GoMemLimit)
		if memGuard.ShouldDowngrade {
			pterm.Warning.Println("可用内存不足 2GB，将自动降低测试数据尺寸")
		}
	} else {
		pterm.Warning.Println("无法检测内存状态，使用保守限制 GOMEMLIMIT=2GiB")
	}

	pkgs := packages
	if *skipL4 {
		pkgs = pkgs[:len(pkgs)-1]
		pterm.Info.Println("跳过 L4/L5 集成测试（--skip-integration）")
	}

	// 显示测试计划
	pkgTableData := pterm.TableData{{"层级", "包路径", "描述"}}
	for _, pkg := range pkgs {
		pkgTableData = append(pkgTableData, []string{pkg.Layer, pkg.Dir, pkg.Description})
	}
	pterm.DefaultTable.WithHasHeader().WithData(pkgTableData).Render()
	pterm.Println()

	cal := runCalibration(root)
	benchDescriptions = applyCalibration(benchDescriptions, cal)

	pterm.Println()
	startTime := time.Now()

	results, cpuInfo, goosInfo, err := runBenchmarks(root, pkgs, *benchtime, *pkgTimeout, memGuard)
	if err != nil {
		pterm.Error.Printf("运行基准测试失败: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime)

	if len(results) == 0 {
		pterm.Error.Println("未收集到任何基准测试结果")
		os.Exit(1)
	}

	// 生成报告
	spinner, _ := pterm.DefaultSpinner.Start("生成报告...")
	layers := groupByLayer(results)
	history, histErr := loadHistory(root)
	hasHistory := histErr == nil && history != nil && len(history.Results) > 0
	categoryGroups := groupByCategory(results)
	overallScore := calcOverallScore(results)
	var historyResults []benchResult
	if hasHistory {
		historyResults = history.Results
	}
	insights := generateInsights(results, historyResults)

	data := &reportData{
		Timestamp:      time.Now().Format("2006-01-02 15:04:05"),
		GoVersion:      runtime.Version(),
		OS:             goosInfo,
		Arch:           runtime.GOARCH,
		CPU:            cpuInfo,
		Results:        results,
		Layers:         layers,
		CategoryGroups: categoryGroups,
		OverallScore:   overallScore,
		Insights:       insights,
		TotalTime:      formatElapsed(elapsed),
		PassCount:      len(results),
		HasHistory:     hasHistory,
		HistoryTime:    "",
		HistoryCount:   0,
		Calibration:    cal,
	}
	if hasHistory {
		data.HistoryTime = history.Timestamp
		data.HistoryCount = len(history.Results)
		data.HistoryResults = history.Results
	}

	if err := ensureBenchDir(root); err != nil {
		pterm.Error.Printf("创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	absOutput := *output
	if absOutput == "" {
		absOutput = filepath.Join(benchDir(root), "bench-report.html")
	} else if !filepath.IsAbs(absOutput) {
		absOutput = filepath.Join(root, absOutput)
	}
	outDir := filepath.Dir(absOutput)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		pterm.Error.Printf("创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	if err := generateReport(data, absOutput); err != nil {
		pterm.Error.Printf("生成报告失败: %v\n", err)
		os.Exit(1)
	}
	spinner.Success("报告生成完成")

	// 保存历史
	if !*noSave {
		newHistory := &historyFile{
			Timestamp: data.Timestamp,
			CPU:       cpuInfo,
			OS:        goosInfo,
			Arch:      runtime.GOARCH,
			GoVersion: runtime.Version(),
			Results:   results,
		}
		if err := saveHistory(root, newHistory); err != nil {
			pterm.Warning.Printf("保存历史缓存失败: %v\n", err)
		} else {
			pterm.Success.Println("历史缓存已保存")
		}
	}

	// 结果摘要
	pterm.Println()
	resultBox := pterm.DefaultBox.WithTitle("测试结果摘要")
	summary := fmt.Sprintf(
		"总耗时: %s\n测试项: %d\n综合评分: %.1f/100\n报告路径: %s",
		data.TotalTime, data.PassCount, data.OverallScore, absOutput,
	)
	if hasHistory {
		summary += fmt.Sprintf("\n历史对比: %s (%d 项)", data.HistoryTime, data.HistoryCount)
	}
	resultBox.Println(summary)

	// 🆕 --store flag：把结果写入 SQLite performance_metrics 表
	if *storePath != "" {
		store, err := sqlite.New(*storePath)
		if err != nil {
			log.Printf("Warning: open sqlite store failed: %v\n", err)
		} else {
			defer store.Close()
			for _, r := range results {
				metrics := performance.PerformanceMetrics{
					TaskID:          fmt.Sprintf("bench-%s-%d", r.Name, time.Now().Unix()),
					TaskType:        "bench",
					PluginName:      r.Layer,
					SourceSize:      r.BytesPerOp * r.IterCount,
					AvgThroughput:   r.MBPerSec,
					PeakThroughput:  r.MBPerSec,
					TotalDurationMs: int64(r.NsPerOp * float64(r.IterCount) / 1e6),
					Grade:           performance.GradeGood, // bench 结果默认 good
					GradeScore:      50,
					CPUScore:        cal.CPUScore,
					CPULabel:        cal.CPULabel,
					CreatedAt:       time.Now(),
				}
				if err := store.SaveMetrics(metrics); err != nil {
					log.Printf("Warning: save metrics for %s failed: %v\n", r.Name, err)
				}
			}
			// 保存校准结果
			calResult := performance.CalibrationResult{
				CPUScore:      cal.CPUScore,
				AESThroughput: cal.AESThroughput,
				CPULabel:      cal.CPULabel,
				CalibratedAt:  time.Now(),
				GoVersion:     runtime.Version(),
				OS:            runtime.GOOS,
				Arch:          runtime.GOARCH,
				NumCPU:        runtime.NumCPU(),
			}
			if err := store.SaveCalibration(calResult); err != nil {
				log.Printf("Warning: save calibration failed: %v\n", err)
			}
			fmt.Printf("✓ Results saved to SQLite: %s\n", *storePath)
		}
	}

	if *openBrowser {
		openInBrowser(absOutput)
	}
}

func openInBrowser(path string) {
	abs, _ := filepath.Abs(path)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", abs)
	case "darwin":
		cmd = exec.Command("open", abs)
	default:
		cmd = exec.Command("xdg-open", abs)
	}
	cmd.Start()
}
