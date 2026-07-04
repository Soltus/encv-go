package register

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/gin-gonic/gin"
)

func StartHttpHandlerWithRetry(handler http.Handler, initialPort int, instanceID, version string) (*http.Server, string, error) {
	maxTries := 100
	for i := 0; i < maxTries; i++ {
		currentPort := initialPort + i
		addr := fmt.Sprintf(":%d", currentPort)

		listener, err := net.Listen("tcp", addr)
		if err != nil {
			logAndContinue(err, currentPort)
			continue
		}

		srv := &http.Server{Handler: handler}
		go func() {
			slog.Info("Backend attempting to start", "addr", addr)
			if serveErr := srv.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
				slog.Error("Backend server encountered an error", "addr", listener.Addr().String(), "error", serveErr)
			}
		}()

		time.Sleep(150 * time.Millisecond)

		if err := performPingCheck(currentPort, instanceID, version); err != nil {
			slog.Warn("Backend self-check failed, trying next port", "port", currentPort, "error", err)
			listener.Close()
			continue
		}

		actualAddr := listener.Addr().String()
		slog.Info("Backend server successfully started", "addr", actualAddr)
		return srv, actualAddr, nil
	}

	return nil, "", fmt.Errorf("failed to start http.Handler after %d tries", maxTries)
}

func StartGinWithRetry(engine *gin.Engine, initialPort int, instanceID, version string) (*http.Server, string, error) {
	handler := engine
	maxTries := 100
	for i := 0; i < maxTries; i++ {
		currentPort := initialPort + i
		addr := fmt.Sprintf(":%d", currentPort)

		listener, err := net.Listen("tcp", addr)
		if err != nil {
			logAndContinue(err, currentPort)
			continue
		}

		srv := &http.Server{Handler: handler}
		go func() {
			slog.Info("Server attempting to start", "addr", addr)
			if serveErr := srv.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
				slog.Error("Server encountered an error", "addr", listener.Addr().String(), "error", serveErr)
			}
		}()

		time.Sleep(150 * time.Millisecond)

		if err := performPingCheck(currentPort, instanceID, version); err != nil {
			slog.Warn("Self-check failed, trying next port", "port", currentPort, "error", err)
			listener.Close()
			continue
		}

		actualAddr := listener.Addr().String()
		slog.Info("Server successfully started", "addr", actualAddr)
		return srv, actualAddr, nil
	}

	return nil, "", fmt.Errorf("failed to start server after %d tries", maxTries)
}

func performPingCheck(port int, expectedInstanceID, expectedVersion string) error {
	pingURL := fmt.Sprintf("http://127.0.0.1:%d/ping", port)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(pingURL)
	if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping request failed: %w", err)
	}
	defer resp.Body.Close()

	var pingResp types.PingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pingResp); err != nil {
		return fmt.Errorf("could not decode ping response: %w", err)
	}

	if pingResp.InstanceID != expectedInstanceID {
		return fmt.Errorf("instance ID mismatch: expected %s, got %s", expectedInstanceID, pingResp.InstanceID)
	}

	if pingResp.Version != expectedVersion {
		slog.Warn("Version mismatch on port", "port", port, "expected", expectedVersion, "got", pingResp.Version)
	}

	return nil
}

func logAndContinue(err error, port int) {
	if utils.IsAddrInUseErr(err) {
		slog.Warn("Port in use, trying next", "port", port)
	} else {
		slog.Warn("Failed to start on port, trying next", "port", port, "error", err)
	}
}
