// pkg/encv/analyze_v2.go

package encv

import (
	"context"

	"github.com/Soltus/encv-go/internal/v2/container/detector"
)

// AnalyzeContainerV2 可视化分析 ENCV 容器文件的结构（v2 架构版本）。
// 注意：此函数名中的 "V2" 指 v2 内部架构（非容器格式版本），支持 ECv2/ECv3/ECv4 格式的检测与分析。
// printToStdout: 是否同时打印到标准输出（为了兼容CLI调用）
// 返回格式化的HTML内容和错误
func AnalyzeContainerV2(ctx context.Context, containerPath string, printToStdout bool) (string, error) {
	return detector.AnalyzeContainerV2(ctx, containerPath, printToStdout)
}
