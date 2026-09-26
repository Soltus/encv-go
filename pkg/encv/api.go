// pkg/encv/api.go
package encv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/plugins"
)

// Init 初始化 ENCV 库所需的所有内部组件。
// 它必须在调用任何其他 ENCV 功能之前被调用。
// 它接受一个 context.Context，以便在初始化期间传递必要的配置。
func Init(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	// 2. 使用合并器，将用户配置与所有插件的默认配置合并
	fullPluginSettings, err := plugins.BuildFullPluginSettings(cfg.PluginSettings)
	if err != nil {
		return fmt.Errorf("failed to build full plugin settings: %w", err)
	}

	// 3. 用合并后的完整设置替换原始设置
	cfg.PluginSettings = fullPluginSettings

	// 4. 按装配配置装载插件（plugin_assembly.enabled 为空 = 全部启用）
	//
	// 错误分级：装配错误（插件名写错 / 依赖缺失 / 成环）必须让调用方失败——
	// 这是配置写错，继续跑只会得到一个说不清的半初始化状态。
	// 单个插件 Initialize 失败仍然不致命：ENCV 的既有哲学是「失败插件被禁用，
	// 服务照常启动」，这里只记日志，状态可在 GetPluginStates 里看到。
	if err := plugins.InitializePluginsWith(ctx, cfg.PluginAssembly.Enabled); err != nil {
		if isAssemblyError(err) {
			return fmt.Errorf("plugin assembly: %w", err)
		}
		slog.Warn("some plugins failed to initialize; they are disabled", "error", err)
	}
	return nil
}

// isAssemblyError 判断错误是否属于「装配配置写错」这类必须让启动失败的问题。
func isAssemblyError(err error) bool {
	return errors.Is(err, plugins.ErrUnknownPlugin) ||
		errors.Is(err, plugins.ErrMissingDependency) ||
		errors.Is(err, plugins.ErrDependencyCycle)
}
