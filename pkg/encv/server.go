package encv

import (
	"context"
	"flag"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/register"
	"github.com/Soltus/encv-go/internal/server"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// 暂时注释，需要修改，勿删

// func StartWebdav(ctx context.Context) (string, string, error) {
// 	return server.StartWebdav(ctx)
// }

func FindServer(startPort int, maxTries int) (string, *types.PingResponse, error) {
	return register.FindServer(startPort, maxTries)
}

// NewPlayer 创建一个新的播放器实例
func NewServer(ctx context.Context, configPath string) *server.Server {
	return server.NewServer(ctx, configPath)
}

// 解析服务标志的辅助函数
func ParseServerFlags(cmd *flag.FlagSet, cfg *config.Config, args []string) error {
	cmd.StringVar(&cfg.Password, "p", cfg.Password, "Password for decryption, overrides config file")
	cmd.StringVar(&cfg.Server.Dir, "d", cfg.Server.Dir, "Directory to serve from, overrides config file")
	cmd.IntVar(&cfg.Server.Port, "port", cfg.Server.Port, "Port to run the server on, overrides config file")
	return cmd.Parse(args)
}
