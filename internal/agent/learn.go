package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// LearnPhase represents the phases of the learn workflow.
type LearnPhase int

const (
	// PhaseMap — PM maps the project structure.
	PhaseMap LearnPhase = iota
	// PhaseExplore — Technical agents explore in parallel.
	PhaseExplore
	// PhaseSynthesize — Lead synthesizes to global.md.
	PhaseSynthesize
)

// LearnConfig configures the learn workflow.
type LearnConfig struct {
	Channel string // Slack channel for the learn thread
	Thread  string // Thread ID
	IsRelearn bool // true if re-learn (compare with existing)
}

// LearnExplorer defines what each agent does during the learn workflow.
type LearnExplorer interface {
	Explore(ctx context.Context, projectMap string, channel, thread string) (string, error)
}

// LearnSynthesizer creates global.md from all agent findings.
type LearnSynthesizer interface {
	Synthesize(ctx context.Context, findings map[string]string, channel, thread string) (string, error)
}

// AgentExploration records what an agent found during exploration.
type AgentExploration struct {
	Role     string        `json:"role"`
	Findings string        `json:"findings"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// LearnResult is the output of a learn workflow run.
type LearnResult struct {
	Explorations []AgentExploration `json:"explorations"`
	GlobalMD     string             `json:"global_md"`
	IsRelearn    bool               `json:"is_relearn"`
	Duration     time.Duration      `json:"duration"`
}

// LearnWorkflow coordinates the learn/onboarding process.
type LearnWorkflow struct {
	pm          LearnExplorer
	explorers   map[string]LearnExplorer // role → explorer
	synthesizer LearnSynthesizer
}

// NewLearnWorkflow creates a new learn workflow coordinator.
func NewLearnWorkflow(pm LearnExplorer, explorers map[string]LearnExplorer, synthesizer LearnSynthesizer) *LearnWorkflow {
	return &LearnWorkflow{
		pm:          pm,
		explorers:   explorers,
		synthesizer: synthesizer,
	}
}

// Run executes the full learn workflow:
// Phase 1: PM maps the project
// Phase 2: Technical agents explore in parallel
// Phase 3: Lead synthesizes to global.md
func (lw *LearnWorkflow) Run(ctx context.Context, cfg LearnConfig) (*LearnResult, error) {
	start := time.Now()
	result := &LearnResult{IsRelearn: cfg.IsRelearn}

	// Phase 1: PM maps the project
	pmFindings, err := lw.runPhaseMap(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("phase 1 (map): %w", err)
	}

	result.Explorations = append(result.Explorations, AgentExploration{
		Role:     "pm",
		Findings: pmFindings,
		Duration: time.Since(start),
	})

	// Phase 2: Technical agents explore in parallel
	phaseStart := time.Now()
	explorations, err := lw.runPhaseExplore(ctx, cfg, pmFindings)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (explore): %w", err)
	}

	for _, exp := range explorations {
		exp.Duration = time.Since(phaseStart)
		result.Explorations = append(result.Explorations, exp)
	}

	// Phase 3: Lead synthesizes
	findings := make(map[string]string)
	findings["pm"] = pmFindings
	for _, exp := range explorations {
		if exp.Error == "" {
			findings[exp.Role] = exp.Findings
		}
	}

	globalMD, err := lw.runPhaseSynthesize(ctx, cfg, findings)
	if err != nil {
		return nil, fmt.Errorf("phase 3 (synthesize): %w", err)
	}

	result.GlobalMD = globalMD
	result.Duration = time.Since(start)

	return result, nil
}

// runPhaseMap executes Phase 1: PM maps the project.
func (lw *LearnWorkflow) runPhaseMap(ctx context.Context, cfg LearnConfig) (string, error) {
	return lw.pm.Explore(ctx, "", cfg.Channel, cfg.Thread)
}

// runPhaseExplore executes Phase 2: agents explore in parallel.
func (lw *LearnWorkflow) runPhaseExplore(ctx context.Context, cfg LearnConfig, pmMap string) ([]AgentExploration, error) {
	explorations := make([]AgentExploration, len(lw.explorers))

	roles := make([]string, 0, len(lw.explorers))
	for role := range lw.explorers {
		roles = append(roles, role)
	}

	g, gctx := errgroup.WithContext(ctx)

	for i, role := range roles {
		explorer := lw.explorers[role]
		g.Go(func() error {
			findings, err := explorer.Explore(gctx, pmMap, cfg.Channel, cfg.Thread)
			if err != nil {
				explorations[i] = AgentExploration{
					Role:  roles[i],
					Error: err.Error(),
				}
				return nil // don't cancel others
			}
			explorations[i] = AgentExploration{
				Role:     roles[i],
				Findings: findings,
			}
			return nil
		})
	}

	g.Wait()

	return explorations, nil
}

// runPhaseSynthesize executes Phase 3: Lead synthesizes.
func (lw *LearnWorkflow) runPhaseSynthesize(ctx context.Context, cfg LearnConfig, findings map[string]string) (string, error) {
	return lw.synthesizer.Synthesize(ctx, findings, cfg.Channel, cfg.Thread)
}

// NeedsLearn detects if a learn workflow should auto-trigger.
// Returns true if the repo has code files but no global.md content.
func NeedsLearn(hasCodeFiles bool, globalMDContent string) bool {
	if !hasCodeFiles {
		return false // fresh repo, nothing to learn
	}
	return strings.TrimSpace(globalMDContent) == ""
}

// FormatLearnPrompt builds the exploration prompt for an agent.
func FormatLearnPrompt(role, pmMap string, isRelearn bool) string {
	var b strings.Builder

	if isRelearn {
		b.WriteString("Re-learn: compare existing project map with current codebase. ")
		b.WriteString("Remove outdated info, update changed info, add new info. ")
		b.WriteString("Result should reflect the project as it is now.\n\n")
	} else {
		b.WriteString("Initial onboarding: explore the codebase from your perspective.\n\n")
	}

	if pmMap != "" {
		b.WriteString("PM's project map (use as starting point):\n")
		b.WriteString(pmMap)
		b.WriteString("\n\n")
	}

	switch role {
	case "pm":
		b.WriteString("Map the project: structure, README, entry points, features, domains. ")
		b.WriteString("Focus on what the project does, not how it's built.")
	case "coder":
		b.WriteString("Explore: architecture, patterns, conventions, build system, test framework, dependencies. ")
		b.WriteString("Focus on how to build on this codebase.")
	case "reviewer":
		b.WriteString("Explore: test coverage, CI config, linting, security patterns, quality hotspots. ")
		b.WriteString("Focus on what to watch for during code review.")
	case "artist":
		b.WriteString("Explore: UI components, design system, styles, screens, responsive patterns. ")
		b.WriteString("Focus on visual consistency and UX patterns.")
	case "lead":
		b.WriteString("Synthesize all agent findings into global.md: architecture, tech stack, conventions, key decisions.")
	}

	return b.String()
}

// FormatSynthesisPrompt builds the prompt for Lead to synthesize global.md.
func FormatSynthesisPrompt(findings map[string]string) string {
	var b strings.Builder

	b.WriteString("Synthesize these agent findings into a cohesive global.md.\n")
	b.WriteString("Include: architecture, tech stack, conventions, key decisions.\n")
	b.WriteString("Keep it concise and actionable — this is shared context for all agents.\n\n")

	for role, finding := range findings {
		b.WriteString(fmt.Sprintf("## %s findings\n\n%s\n\n", role, finding))
	}

	return b.String()
}

// CompactKnowledge compares old and new knowledge, producing a compacted version.
// Used during re-learn to clean up outdated information.
func CompactKnowledge(existing, fresh string) string {
	if existing == "" {
		return fresh
	}
	if fresh == "" {
		return existing
	}

	// Build a combined prompt for the LLM to handle actual compaction.
	// The caller should feed this to the LLM for intelligent merging.
	var b strings.Builder
	b.WriteString("Compare existing knowledge with fresh findings. Produce updated knowledge:\n")
	b.WriteString("- Remove information that is no longer true\n")
	b.WriteString("- Update information that has changed\n")
	b.WriteString("- Add new information\n")
	b.WriteString("- Result should reflect the project as it is NOW\n\n")
	b.WriteString("## Existing knowledge\n\n")
	b.WriteString(existing)
	b.WriteString("\n\n## Fresh findings\n\n")
	b.WriteString(fresh)

	return b.String()
}
