package children

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"preview-gateway/internal/paths"
)

type Child struct {
	Name     string
	Cmd      string
	Args     []string
	Dir      string
	Env      []string
	Color    string
	Required bool
	ReadyURL string

	cmd    *exec.Cmd
	cancel context.CancelFunc
	mu     sync.Mutex
	alive  bool
}

type Manager struct {
	children []*Child
	paths    *paths.Paths
	wg       sync.WaitGroup
}

func New(p *paths.Paths, spawnGo, spawnVite, spawnPluginVite, spawnOpenlist, spawnSimverseVite bool) *Manager {
	m := &Manager{paths: p}

	if spawnGo {
		m.children = append(m.children, &Child{
			Name:     "encv-go",
			Cmd:      p.AirBin,
			Args:     []string{"--", "--skip-service-guard", "--storage-root", p.MobileDataDir},
			Dir:      p.RepoRoot,
			Color:    "\033[36m",
			Required: true,
			ReadyURL: "http://127.0.0.1:2025/health",
		})
	}

	if spawnVite {
		m.children = append(m.children, &Child{
			Name:     "encv-mobile-vite",
			Cmd:      p.NodeBin,
			Args:     []string{"node_modules/vite/bin/vite.js", "--host", "0.0.0.0", "--port", "8100"},
			Dir:      p.MobileDir,
			Color:    "\033[32m",
			Required: true,
			ReadyURL: "http://127.0.0.1:8100/",
		})
	}

	if spawnPluginVite {
		m.children = append(m.children, &Child{
			Name:     "plugin-openlist-web",
			Cmd:      p.NodeBin,
			Args:     []string{"node_modules/vite/bin/vite.js", "--host", "0.0.0.0", "--port", "5174"},
			Dir:      p.PluginWebDir,
			Color:    "\033[35m",
			Required: false,
			ReadyURL: "http://127.0.0.1:5174/",
		})
	}

	if spawnOpenlist {
		m.children = append(m.children, &Child{
			Name:     "openlist-direct",
			Cmd:      p.NodeBin,
			Args:     []string{"node_modules/vite/bin/vite.js", "--host", "0.0.0.0", "--port", "5244"},
			Dir:      filepath.Join(p.RepoRoot, "app", "openlist"),
			Color:    "\033[33m",
			Required: false,
			ReadyURL: "http://127.0.0.1:5244/",
		})
	}

	if spawnSimverseVite {
		m.children = append(m.children, &Child{
			Name:     "plugin-simverse-web",
			Cmd:      p.NodeBin,
			Args:     []string{"node_modules/vite/bin/vite.js", "--host", "0.0.0.0", "--port", "5176"},
			Dir:      p.SimverseFrontendDir,
			Color:    "\033[34m",
			Required: false,
			ReadyURL: "http://127.0.0.1:5176/",
		})
	}

	return m
}

func (m *Manager) StartAll() error {
	log.Printf("[children] Starting %d child processes...", len(m.children))

	for i, c := range m.children {
		if err := m.startChild(c); err != nil {
			if c.Required {
				return fmt.Errorf("required child %s failed to start: %w", c.Name, err)
			}
			log.Printf("[children] Optional child %s failed to start: %v", c.Name, err)
			continue
		}

		if c.ReadyURL != "" {
			log.Printf("[children] Waiting for %s to be ready...", c.Name)
			if err := waitForReady(c.ReadyURL, 30*time.Second); err != nil {
				if c.Required {
					return fmt.Errorf("required child %s not ready: %w", c.Name, err)
				}
				log.Printf("[children] Optional child %s not ready in time: %v", c.Name, err)
			} else {
				log.Printf("[children] %s is ready", c.Name)
			}
		}

		_ = i
	}

	log.Println("[children] All required children started and ready")
	return nil
}

func (m *Manager) startChild(c *Child) error {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	cmd := exec.CommandContext(ctx, c.Cmd, c.Args...)
	cmd.Dir = c.Dir

	env := os.Environ()
	env = append(env, c.Env...)
	env = append(env, "ENCV_DEV_PREVIEW=1")
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}

	c.cmd = cmd
	c.mu.Lock()
	c.alive = true
	c.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			log.Printf("%s[%s]\033[0m %s", c.Color, c.Name, scanner.Text())
		}
	}()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			log.Printf("%s[%s]\033[0m %s", c.Color, c.Name, scanner.Text())
		}
	}()

	go func() {
		err := cmd.Wait()
		c.mu.Lock()
		c.alive = false
		c.mu.Unlock()
		if err != nil {
			if ctx.Err() == context.Canceled {
				log.Printf("[children] %s exited (cancelled)", c.Name)
			} else {
				log.Printf("[children] %s exited with error: %v", c.Name, err)
			}
		} else {
			log.Printf("[children] %s exited normally", c.Name)
		}
	}()

	log.Printf("[children] Started %s (pid=%d)", c.Name, cmd.Process.Pid)
	return nil
}

func waitForReady(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp != nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

func (m *Manager) StopAll() {
	log.Println("[children] Stopping all children...")

	for _, c := range m.children {
		c.mu.Lock()
		alive := c.alive
		c.mu.Unlock()
		if !alive {
			continue
		}
		log.Printf("[children] Stopping %s...", c.Name)
		if c.cmd != nil && c.cmd.Process != nil {
			c.cmd.Process.Signal(syscall.SIGTERM)
		}
	}

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		done <- struct{}{}
	}()

	select {
	case <-done:
		log.Println("[children] All children stopped gracefully")
	case <-time.After(5 * time.Second):
		log.Println("[children] Graceful stop timeout, force killing...")
		for _, c := range m.children {
			if c.cancel != nil {
				c.cancel()
			}
			if c.cmd != nil && c.cmd.Process != nil {
				c.cmd.Process.Kill()
			}
		}
		<-done
		log.Println("[children] All children killed")
	}
}

func (m *Manager) Status() map[string]bool {
	status := make(map[string]bool)
	for _, c := range m.children {
		c.mu.Lock()
		status[c.Name] = c.alive
		c.mu.Unlock()
	}
	return status
}

func (m *Manager) HasChildren() bool {
	return len(m.children) > 0
}

var colorReset = "\033[0m"

func init() {
	_ = strings.TrimSpace
	_ = colorReset
}
