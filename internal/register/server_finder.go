package register

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// FindServer 尝试在指定端口范围内发现正在运行的 encv 服务器
func FindServer(startPort int, maxTries int) (string, *types.PingResponse, error) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	slog.Info("Discovering ENCV server", "port_start", startPort, "port_end", startPort+maxTries-1)

	for i := 0; i < maxTries; i++ {
		currentPort := startPort + i
		pingURL := fmt.Sprintf("http://127.0.0.1:%d/ping", currentPort)

		resp, err := client.Get(pingURL)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var pingResp types.PingResponse
			if err := json.NewDecoder(resp.Body).Decode(&pingResp); err != nil {
				continue
			}
			if pingResp.Status == types.ServiceStatuses.OK {
				serverAddr := fmt.Sprintf("127.0.0.1:%d", currentPort)
				slog.Info("Found ENCV server", "addr", serverAddr, "instance", pingResp.InstanceID, "version", pingResp.Version)
				return serverAddr, &pingResp, nil
			}
		}
	}

	return "", nil, fmt.Errorf("could not find any running ENCV server in port range %d-%d", startPort, startPort+maxTries-1)
}
