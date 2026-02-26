package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// DefaultToolCallTimeout is the default timeout for MCP tool calls.
const DefaultToolCallTimeout = 30 * time.Second

// ShutdownGracePeriod is how long to wait after SIGTERM before SIGKILL.
const ShutdownGracePeriod = 5 * time.Second

// ServerProcess represents a running MCP server child process.
type ServerProcess struct {
	Name   string
	Config ServerConfig
	Cmd    *exec.Cmd
	Client *Client
	Tools  []MCPTool

	mu     sync.Mutex
	alive  bool
}

// Manager handles the lifecycle of MCP server processes.
type Manager struct {
	servers map[string]*ServerProcess
	role    string
	logger  *slog.Logger
	mu      sync.RWMutex

	toolTimeout time.Duration
}

// ManagerOption configures the Manager.
type ManagerOption func(*Manager)

// WithToolTimeout sets the timeout for MCP tool calls.
func WithToolTimeout(d time.Duration) ManagerOption {
	return func(m *Manager) {
		m.toolTimeout = d
	}
}

// WithManagerLogger sets the logger for the Manager.
func WithManagerLogger(l *slog.Logger) ManagerOption {
	return func(m *Manager) {
		m.logger = l
	}
}

// NewManager creates a new MCP server manager for the given role.
func NewManager(role string, opts ...ManagerOption) *Manager {
	m := &Manager{
		servers:     make(map[string]*ServerProcess),
		role:        role,
		logger:      slog.Default(),
		toolTimeout: DefaultToolCallTimeout,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// ProcessStarter abstracts process creation for testability.
type ProcessStarter interface {
	Start(name string, config ServerConfig) (*exec.Cmd, *Client, error)
}

// defaultStarter starts real OS processes.
type defaultStarter struct{}

func (d *defaultStarter) Start(name string, config ServerConfig) (*exec.Cmd, *Client, error) {
	args := ResolveArgs(config.Args)
	cmd := exec.Command(config.Command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start process: %w", err)
	}

	client := NewClient(stdin, stdout)
	return cmd, client, nil
}

// StartAll launches MCP servers from config, filtered by this manager's role.
// Servers that fail to start are logged and skipped — not fatal.
func (m *Manager) StartAll(ctx context.Context, cfg *MCPConfig) error {
	return m.StartAllWith(ctx, cfg, &defaultStarter{})
}

// StartAllWith launches MCP servers using the provided ProcessStarter.
func (m *Manager) StartAllWith(ctx context.Context, cfg *MCPConfig, starter ProcessStarter) error {
	servers := FilterByRole(cfg, m.role)

	for name, serverCfg := range servers {
		if err := m.startServer(ctx, name, serverCfg, starter); err != nil {
			m.logger.Warn("MCP server failed to start",
				"server", name,
				"error", err,
			)
			continue
		}
	}

	return nil
}

// startServer starts a single MCP server, performs handshake, discovers tools.
func (m *Manager) startServer(ctx context.Context, name string, config ServerConfig, starter ProcessStarter) error {
	cmd, client, err := starter.Start(name, config)
	if err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}

	sp := &ServerProcess{
		Name:   name,
		Config: config,
		Cmd:    cmd,
		Client: client,
		alive:  true,
	}

	// Initialize handshake
	initCtx, cancel := context.WithTimeout(ctx, m.toolTimeout)
	defer cancel()

	_, err = client.Initialize(initCtx)
	if err != nil {
		m.killProcess(sp)
		return fmt.Errorf("initialize %s: %w", name, err)
	}

	// Discover tools
	listCtx, listCancel := context.WithTimeout(ctx, m.toolTimeout)
	defer listCancel()

	tools, err := client.ListTools(listCtx)
	if err != nil {
		m.killProcess(sp)
		return fmt.Errorf("tools/list %s: %w", name, err)
	}

	sp.Tools = tools

	m.mu.Lock()
	m.servers[name] = sp
	m.mu.Unlock()

	m.logger.Info("MCP server started",
		"server", name,
		"tools_discovered", len(tools),
	)

	return nil
}

// CallTool routes a tool call to the correct MCP server.
func (m *Manager) CallTool(ctx context.Context, serverName, toolName string, arguments json.RawMessage) (*ToolCallResult, error) {
	m.mu.RLock()
	sp, ok := m.servers[serverName]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("MCP server %q not found", serverName)
	}

	sp.mu.Lock()
	if !sp.alive {
		sp.mu.Unlock()
		return nil, fmt.Errorf("MCP server %q is not running", serverName)
	}
	sp.mu.Unlock()

	callCtx, cancel := context.WithTimeout(ctx, m.toolTimeout)
	defer cancel()

	result, err := sp.Client.CallTool(callCtx, toolName, arguments)
	if err != nil {
		// Check if server crashed
		if sp.Cmd != nil && sp.Cmd.ProcessState != nil {
			sp.mu.Lock()
			sp.alive = false
			sp.mu.Unlock()

			m.logger.Error("MCP server crashed during tool call",
				"server", serverName,
				"tool", toolName,
				"error", err,
			)

			// Remove tools from this server
			m.removeServer(serverName)
		}
		return nil, err
	}

	return result, nil
}

// AllTools returns all tools from all running MCP servers.
// Returns a map from tool name to server name + MCPTool.
func (m *Manager) AllTools() map[string]MCPToolEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]MCPToolEntry)
	for serverName, sp := range m.servers {
		sp.mu.Lock()
		alive := sp.alive
		sp.mu.Unlock()
		if !alive {
			continue
		}
		for _, tool := range sp.Tools {
			result[tool.Name] = MCPToolEntry{
				ServerName: serverName,
				Tool:       tool,
			}
		}
	}
	return result
}

// MCPToolEntry associates an MCP tool with its owning server.
type MCPToolEntry struct {
	ServerName string
	Tool       MCPTool
}

// StopAll gracefully shuts down all MCP server processes.
func (m *Manager) StopAll() {
	m.mu.Lock()
	servers := make([]*ServerProcess, 0, len(m.servers))
	for _, sp := range m.servers {
		servers = append(servers, sp)
	}
	m.servers = make(map[string]*ServerProcess)
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, sp := range servers {
		wg.Add(1)
		go func(sp *ServerProcess) {
			defer wg.Done()
			m.stopServer(sp)
		}(sp)
	}
	wg.Wait()
}

// stopServer sends SIGTERM, waits for grace period, then SIGKILL.
func (m *Manager) stopServer(sp *ServerProcess) {
	if sp.Cmd == nil || sp.Cmd.Process == nil {
		return
	}

	sp.mu.Lock()
	sp.alive = false
	sp.mu.Unlock()

	sp.Client.Close()

	// Send SIGTERM
	if err := sp.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
		m.logger.Warn("failed to send SIGTERM to MCP server",
			"server", sp.Name,
			"error", err,
		)
		// Try SIGKILL immediately
		_ = sp.Cmd.Process.Kill()
		return
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		_ = sp.Cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("MCP server stopped gracefully", "server", sp.Name)
	case <-time.After(ShutdownGracePeriod):
		m.logger.Warn("MCP server did not stop in time, sending SIGKILL",
			"server", sp.Name,
		)
		_ = sp.Cmd.Process.Kill()
		<-done
	}
}

// killProcess forcefully kills a server process (used during startup failures).
func (m *Manager) killProcess(sp *ServerProcess) {
	if sp.Cmd != nil && sp.Cmd.Process != nil {
		_ = sp.Cmd.Process.Kill()
		_ = sp.Cmd.Wait()
	}
}

// removeServer removes a server from the registry (when it has crashed).
func (m *Manager) removeServer(name string) {
	m.mu.Lock()
	delete(m.servers, name)
	m.mu.Unlock()
}

// ServerCount returns the number of running MCP servers.
func (m *Manager) ServerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.servers)
}

// IsAlive checks if a specific server is still running.
func (m *Manager) IsAlive(name string) bool {
	m.mu.RLock()
	sp, ok := m.servers[name]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.alive
}
