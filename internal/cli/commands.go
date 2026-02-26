// Package cli implements CLI subcommand routing and service management
// for the codebutler binary.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Command represents a CLI subcommand.
type Command struct {
	Name        string
	Description string
	Run         func(args []string) error
}

// Router dispatches subcommands.
type Router struct {
	commands map[string]*Command
}

// NewRouter creates a new CLI router.
func NewRouter() *Router {
	return &Router{
		commands: make(map[string]*Command),
	}
}

// Register adds a command to the router.
func (r *Router) Register(cmd *Command) {
	r.commands[cmd.Name] = cmd
}

// Dispatch routes to the correct command or returns an error.
func (r *Router) Dispatch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	name := args[0]
	cmd, ok := r.commands[name]
	if !ok {
		return fmt.Errorf("unknown command %q", name)
	}

	return cmd.Run(args[1:])
}

// HasCommand checks if a command is registered.
func (r *Router) HasCommand(name string) bool {
	_, ok := r.commands[name]
	return ok
}

// ListCommands returns all registered command names with descriptions.
func (r *Router) ListCommands() []Command {
	var cmds []Command
	for _, cmd := range r.commands {
		cmds = append(cmds, *cmd)
	}
	return cmds
}

// Usage returns usage text for all commands.
func (r *Router) Usage() string {
	var b strings.Builder
	b.WriteString("Commands:\n")
	for _, cmd := range r.commands {
		fmt.Fprintf(&b, "  %-12s %s\n", cmd.Name, cmd.Description)
	}
	return b.String()
}

// AgentRoles returns the list of valid agent roles.
var AgentRoles = []string{"pm", "coder", "reviewer", "researcher", "artist", "lead"}

// ServiceManager manages agent services on the current OS.
type ServiceManager struct {
	binaryPath string
	repoDir    string
}

// NewServiceManager creates a service manager.
func NewServiceManager(binaryPath, repoDir string) *ServiceManager {
	return &ServiceManager{
		binaryPath: binaryPath,
		repoDir:    repoDir,
	}
}

// Start launches all agent services.
func (sm *ServiceManager) Start(roles []string) ([]ServiceStatus, error) {
	var results []ServiceStatus
	for _, role := range roles {
		st := sm.startRole(role)
		results = append(results, st)
	}
	return results, nil
}

// Stop stops all agent services.
func (sm *ServiceManager) Stop(roles []string) ([]ServiceStatus, error) {
	var results []ServiceStatus
	for _, role := range roles {
		st := sm.stopRole(role)
		results = append(results, st)
	}
	return results, nil
}

// Status checks the status of all agent services.
func (sm *ServiceManager) Status(roles []string) []ServiceStatus {
	var results []ServiceStatus
	for _, role := range roles {
		st := sm.checkRole(role)
		results = append(results, st)
	}
	return results
}

// ServiceStatus represents the status of a single agent service.
type ServiceStatus struct {
	Role    string `json:"role"`
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (sm *ServiceManager) startRole(role string) ServiceStatus {
	switch runtime.GOOS {
	case "darwin":
		return sm.launchdStart(role)
	case "linux":
		return sm.systemdStart(role)
	default:
		return ServiceStatus{Role: role, Error: "unsupported OS: " + runtime.GOOS}
	}
}

func (sm *ServiceManager) stopRole(role string) ServiceStatus {
	switch runtime.GOOS {
	case "darwin":
		return sm.launchdStop(role)
	case "linux":
		return sm.systemdStop(role)
	default:
		return ServiceStatus{Role: role, Error: "unsupported OS: " + runtime.GOOS}
	}
}

func (sm *ServiceManager) checkRole(role string) ServiceStatus {
	switch runtime.GOOS {
	case "darwin":
		return sm.launchdStatus(role)
	case "linux":
		return sm.systemdStatus(role)
	default:
		return ServiceStatus{Role: role, Error: "unsupported OS: " + runtime.GOOS}
	}
}

func (sm *ServiceManager) serviceLabel(role string) string {
	return fmt.Sprintf("com.codebutler.%s", role)
}

func (sm *ServiceManager) unitName(role string) string {
	return fmt.Sprintf("codebutler-%s", role)
}

func (sm *ServiceManager) launchdStart(role string) ServiceStatus {
	label := sm.serviceLabel(role)
	cmd := exec.Command("launchctl", "start", label)
	if err := cmd.Run(); err != nil {
		return ServiceStatus{Role: role, Error: err.Error()}
	}
	return ServiceStatus{Role: role, Running: true}
}

func (sm *ServiceManager) launchdStop(role string) ServiceStatus {
	label := sm.serviceLabel(role)
	cmd := exec.Command("launchctl", "stop", label)
	if err := cmd.Run(); err != nil {
		return ServiceStatus{Role: role, Error: err.Error()}
	}
	return ServiceStatus{Role: role, Running: false}
}

func (sm *ServiceManager) launchdStatus(role string) ServiceStatus {
	label := sm.serviceLabel(role)
	cmd := exec.Command("launchctl", "list", label)
	out, err := cmd.Output()
	if err != nil {
		return ServiceStatus{Role: role, Running: false}
	}
	return ServiceStatus{Role: role, Running: strings.Contains(string(out), label)}
}

func (sm *ServiceManager) systemdStart(role string) ServiceStatus {
	unit := sm.unitName(role)
	cmd := exec.Command("systemctl", "--user", "start", unit)
	if err := cmd.Run(); err != nil {
		return ServiceStatus{Role: role, Error: err.Error()}
	}
	return ServiceStatus{Role: role, Running: true}
}

func (sm *ServiceManager) systemdStop(role string) ServiceStatus {
	unit := sm.unitName(role)
	cmd := exec.Command("systemctl", "--user", "stop", unit)
	if err := cmd.Run(); err != nil {
		return ServiceStatus{Role: role, Error: err.Error()}
	}
	return ServiceStatus{Role: role, Running: false}
}

func (sm *ServiceManager) systemdStatus(role string) ServiceStatus {
	unit := sm.unitName(role)
	cmd := exec.Command("systemctl", "--user", "is-active", unit)
	out, err := cmd.Output()
	if err != nil {
		return ServiceStatus{Role: role, Running: false}
	}
	return ServiceStatus{Role: role, Running: strings.TrimSpace(string(out)) == "active"}
}

// FormatStatus formats a list of service statuses for display.
func FormatStatus(statuses []ServiceStatus) string {
	var b strings.Builder
	for _, s := range statuses {
		icon := "stopped"
		if s.Running {
			icon = "running"
		}
		line := fmt.Sprintf("  %-12s %s", s.Role, icon)
		if s.PID > 0 {
			line += fmt.Sprintf(" (pid %d)", s.PID)
		}
		if s.Error != "" {
			line += fmt.Sprintf(" [error: %s]", s.Error)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// FindBinary locates the codebutler binary path.
func FindBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find executable: %w", err)
	}
	return filepath.EvalSymlinks(exe)
}

// FindRepoDir walks up from cwd looking for .codebutler/.
func FindRepoDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".codebutler")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .codebutler/ found (searched from %s to root)", dir)
		}
		dir = parent
	}
}
