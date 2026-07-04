// pkg/encv/api.go
package encv

import (
	"context"
	"fmt"

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
	plugins.InitializePlugins(ctx)
	return nil
}
