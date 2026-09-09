package roles

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nimsforest/nimsforestmastery/internal/soil"
)

// Soil key prefixes the portal reads. The two-segment skill key shape
// is canonical: catalog.skills.<agent_type>.<skill_name>.
const (
	catalogNimsPrefix   = "catalog.nims."
	promptsNimsPrefix   = "prompts.nims."
	configNimsPrefix    = "config.nims."
	catalogToolsPrefix  = "catalog.tools."
	catalogSkillsPrefix = "catalog.skills."
	skillsContentPrefix = "skills.content."

	// configForestAgentType is the org default agent type, the middle
	// layer of the runtime fallback chain in
	// nimsforest2 pkg/runtime/agentroutertreehouse.go resolveBackend.
	configForestAgentType = "config.forest.agent_type"

	// agenthumanConfigKey names the backing humans per nim:
	// humans.<name>.backs, consumed by getBackingHumans in
	// nimsforest2 pkg/runtime/nim.go.
	agenthumanConfigKey = "agenthuman.config"
)

// roleNamePattern is the strict role name charset. A role name is a
// nim name from the catalog; anything outside this set can never
// match a nim and must not reach a request path.
var roleNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

// ValidRoleName reports whether a requested role name is safe to use
// in Soil keys and in nimregistry request paths. It rejects path
// separators, query and fragment characters, and dot-dot segments, so
// a crafted name cannot forge a request to another path.
func ValidRoleName(name string) bool {
	return len(name) <= 128 &&
		roleNamePattern.MatchString(name) &&
		!strings.Contains(name, "..")
}

// Service generates role summaries and bundles. It reads Soil first
// and falls back to the nimregistry HTTP API. Soil-served renderings
// are cached; Soil watches invalidate the cache.
type Service struct {
	soil     soil.Reader
	registry *RegistryClient

	mu        sync.RWMutex
	bundles   map[string]*RoleBundle
	list      []RoleSummary
	listValid bool

	// gen counts invalidations. An assemble that started before an
	// invalidation must not enter the cache after it: the generation
	// is captured before the Soil reads and checked at insert time.
	gen uint64

	// cacheDisabled turns the service into a pure read-through. Set
	// when the Soil watches could not start: without invalidation a
	// cached render could silently go stale, which the anti-drift
	// rule forbids.
	cacheDisabled bool
}

// NewService builds a Service over a Soil reader and the nimregistry
// fallback client.
func NewService(reader soil.Reader, registry *RegistryClient) *Service {
	return &Service{
		soil:     reader,
		registry: registry,
		bundles:  make(map[string]*RoleBundle),
	}
}

// Degraded reports whether the service runs without Soil.
func (s *Service) Degraded() bool { return s.soil.Disabled() }

// Wire shapes of the Soil values this service reads.
type catalogNim struct {
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	Description     string   `json:"description"`
	Category        string   `json:"category"`
	LongDescription string   `json:"long_description,omitempty"`
	Subjects        []string `json:"subjects,omitempty"`
}

type nimConfig struct {
	AgentType string `json:"agent_type"`
	Model     string `json:"model"`
	Provider  string `json:"provider"`
}

type toolEntry struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	AssignedNims []string `json:"assigned_nims"`
}

type skillEntry struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	AgentType    string   `json:"agent_type"`
	Path         string   `json:"path"`
	Excerpt      string   `json:"excerpt"`
	AssignedNims []string `json:"assigned_nims"`
}

// ListRoles enumerates the live role directory. The only source of
// enumeration is catalog.nims.*; there is no authored role list.
func (s *Service) ListRoles() ([]RoleSummary, error) {
	s.mu.RLock()
	if s.listValid {
		list := s.list
		s.mu.RUnlock()
		return list, nil
	}
	gen := s.gen
	s.mu.RUnlock()

	entries, err := s.soil.List(catalogNimsPrefix)
	if errors.Is(err, soil.ErrDisabled) {
		return s.listRolesFromRegistry()
	}
	if err != nil {
		return nil, err
	}

	summaries := make([]RoleSummary, 0, len(entries))
	for _, entry := range entries {
		var nim catalogNim
		if err := json.Unmarshal(entry.Value, &nim); err != nil {
			continue // one bad entry must not hide the catalog
		}
		summaries = append(summaries, RoleSummary{
			Name:        strings.TrimPrefix(entry.Key, catalogNimsPrefix),
			Role:        nim.Role,
			Description: nim.Description,
			Category:    nim.Category,
			Source:      SourceRef{Key: entry.Key, Revision: entry.Revision, Origin: OriginSoil},
		})
	}

	s.mu.Lock()
	// Insert only when no invalidation fired during the Soil reads;
	// a lost invalidation would cache a stale list.
	if !s.cacheDisabled && s.gen == gen {
		s.list = summaries
		s.listValid = true
	}
	s.mu.Unlock()
	return summaries, nil
}

func (s *Service) listRolesFromRegistry() ([]RoleSummary, error) {
	nims, err := s.registry.ListNims()
	if err != nil {
		return nil, fmt.Errorf("%w: soil disabled and registry fallback failed: %v", ErrUnavailable, err)
	}
	summaries := make([]RoleSummary, 0, len(nims))
	for _, nim := range nims {
		summaries = append(summaries, RoleSummary{
			Name:        nim.Name,
			Role:        nim.Role,
			Description: nim.Description,
			Category:    nim.Category,
			Source:      SourceRef{Key: catalogNimsPrefix + nim.Name, Revision: 0, Origin: OriginRegistry},
		})
	}
	return summaries, nil
}

// Bundle generates the six-slot bundle for one role. Soil-served
// bundles are cached until a watch invalidates them; fallback bundles
// are never cached.
func (s *Service) Bundle(name string) (*RoleBundle, error) {
	// A role name outside the strict charset can never be a nim, and
	// it must not reach a Soil key or a registry request path.
	if !ValidRoleName(name) {
		return nil, ErrRoleNotFound
	}

	s.mu.RLock()
	if bundle, ok := s.bundles[name]; ok {
		s.mu.RUnlock()
		return bundle, nil
	}
	gen := s.gen
	s.mu.RUnlock()

	bundle, err := s.assemble(name)
	if err != nil {
		return nil, err
	}
	if !bundle.Degraded {
		s.mu.Lock()
		// Insert only when no invalidation fired during assemble; a
		// lost invalidation would cache a stale bundle.
		if !s.cacheDisabled && s.gen == gen {
			s.bundles[name] = bundle
		}
		s.mu.Unlock()
	}
	return bundle, nil
}

func (s *Service) assemble(name string) (*RoleBundle, error) {
	identityEntry, err := s.soil.Get(catalogNimsPrefix + name)
	if errors.Is(err, soil.ErrDisabled) {
		return s.assembleFromRegistry(name)
	}
	if errors.Is(err, soil.ErrNotFound) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}

	var nim catalogNim
	if err := json.Unmarshal(identityEntry.Value, &nim); err != nil {
		return nil, fmt.Errorf("roles: parse %s: %w", identityEntry.Key, err)
	}

	identity := Identity{
		Name:            name,
		Role:            nim.Role,
		Description:     nim.Description,
		Category:        nim.Category,
		LongDescription: nim.LongDescription,
		Subjects:        nim.Subjects,
		Source:          SourceRef{Key: identityEntry.Key, Revision: identityEntry.Revision, Origin: OriginSoil},
	}

	promptKey := promptsNimsPrefix + name
	promptEntry, err := s.soil.Get(promptKey)
	switch {
	case err == nil:
		identity.Prompt = string(promptEntry.Value)
		identity.PromptSource = SourceRef{Key: promptKey, Revision: promptEntry.Revision, Origin: OriginSoil}
	case errors.Is(err, soil.ErrNotFound):
		identity.PromptSource = SourceRef{Key: promptKey}
	default:
		return nil, err
	}

	identity.CeremonyPrompts, err = s.buildCeremonyPrompts(name)
	if err != nil {
		return nil, err
	}

	identity.BackingHumans, identity.BackingSource, err = s.buildBackingHumans(name)
	if err != nil {
		return nil, err
	}

	policy, err := s.buildPolicy(name)
	if err != nil {
		return nil, err
	}

	skills, err := s.buildSkills(name)
	if err != nil {
		return nil, err
	}

	return &RoleBundle{
		Name:        name,
		GeneratedAt: time.Now().UTC(),
		Identity:    identity,
		Policy:      policy,
		Skills:      skills,
		Memory:      Memory{Namespace: memoryNamespace(name)},
		Docs:        s.buildDocs(name),
		Acceptance:  BuildAcceptance(nim.LongDescription, identity.Source),
		Degraded:    false,
	}, nil
}

// buildCeremonyPrompts reads the ceremony subkeys of the behavior
// slot: prompts.nims.<name>.audit, .awaken, .autonomate.*, .eos and
// any other subkey the runtime adds. The list is prefix driven, so a
// new ceremony appears in the bundle without a portal change. Each
// entry carries its own Soil revision stamp.
func (s *Service) buildCeremonyPrompts(name string) ([]CeremonyPrompt, error) {
	prefix := promptsNimsPrefix + name + "."
	entries, err := s.soil.List(prefix)
	if err != nil {
		return nil, err
	}
	var prompts []CeremonyPrompt
	for _, entry := range entries {
		prompts = append(prompts, CeremonyPrompt{
			Name:    strings.TrimPrefix(entry.Key, prefix),
			Content: string(entry.Value),
			Source:  SourceRef{Key: entry.Key, Revision: entry.Revision, Origin: OriginSoil},
		})
	}
	return prompts, nil
}

// buildBackingHumans reads agenthuman.config and returns the human
// names whose backs list contains this nim, the same shape
// getBackingHumans reads in nimsforest2 pkg/runtime/nim.go.
func (s *Service) buildBackingHumans(name string) ([]string, SourceRef, error) {
	entry, err := s.soil.Get(agenthumanConfigKey)
	switch {
	case errors.Is(err, soil.ErrNotFound):
		return nil, SourceRef{Key: agenthumanConfigKey}, nil
	case err != nil:
		return nil, SourceRef{}, err
	}
	source := SourceRef{Key: entry.Key, Revision: entry.Revision, Origin: OriginSoil}
	var cfg struct {
		Humans map[string]struct {
			Backs []string `json:"backs"`
		} `json:"humans"`
	}
	if json.Unmarshal(entry.Value, &cfg) != nil {
		// A malformed config must not fail the bundle; the changelog
		// still shows the key and revision.
		return nil, source, nil
	}
	var names []string
	for human, h := range cfg.Humans {
		for _, backed := range h.Backs {
			if backed == name {
				names = append(names, human)
				break
			}
		}
	}
	sort.Strings(names)
	return names, source, nil
}

func (s *Service) buildPolicy(name string) (Policy, error) {
	policy := Policy{
		AgentType:    DefaultAgentType,
		Tools:        []ToolAssignment{},
		ConfigSource: SourceRef{Key: configNimsPrefix + name},
	}

	// The runtime fallback chain (resolveBackend in nimsforest2
	// pkg/runtime/agentroutertreehouse.go) is per-nim config, then
	// the org default config.forest.agent_type, then "claudecode".
	if orgDefault, err := s.orgDefaultAgentType(); err != nil {
		return Policy{}, err
	} else if orgDefault != "" {
		policy.AgentType = orgDefault
	}

	configEntry, err := s.soil.Get(configNimsPrefix + name)
	switch {
	case err == nil:
		var cfg nimConfig
		if jsonErr := json.Unmarshal(configEntry.Value, &cfg); jsonErr == nil {
			if cfg.AgentType != "" {
				policy.AgentType = cfg.AgentType
			}
			policy.Model = cfg.Model
			policy.Provider = cfg.Provider
		}
		policy.ConfigSource = SourceRef{Key: configEntry.Key, Revision: configEntry.Revision, Origin: OriginSoil}
	case errors.Is(err, soil.ErrNotFound):
		// No per-nim config: the org default or runtime default applies.
	default:
		return Policy{}, err
	}

	toolEntries, err := s.soil.List(catalogToolsPrefix)
	if err != nil {
		return Policy{}, err
	}
	return s.appendToolAssignments(policy, toolEntries, name)
}

// orgDefaultAgentType reads config.forest.agent_type, the middle
// layer of the runtime fallback chain. The runtime treats an empty
// value and the legacy "ai" value as no override, and the stored
// value may carry JSON quotes.
func (s *Service) orgDefaultAgentType() (string, error) {
	entry, err := s.soil.Get(configForestAgentType)
	if errors.Is(err, soil.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(entry.Value))
	if strings.HasPrefix(value, `"`) {
		var unquoted string
		if json.Unmarshal(entry.Value, &unquoted) == nil {
			value = unquoted
		}
	}
	if value == "" || value == "ai" {
		return "", nil
	}
	return value, nil
}

// appendToolAssignments adds the tools whose assigned_nims contain
// this role to the policy slot.
func (s *Service) appendToolAssignments(policy Policy, toolEntries []soil.Entry, name string) (Policy, error) {
	for _, entry := range toolEntries {
		var t toolEntry
		if json.Unmarshal(entry.Value, &t) != nil {
			continue
		}
		if !assignedTo(t.AssignedNims, name) {
			continue
		}
		policy.Tools = append(policy.Tools, ToolAssignment{
			Name:        t.Name,
			Description: t.Description,
			Source:      SourceRef{Key: entry.Key, Revision: entry.Revision, Origin: OriginSoil},
		})
	}
	return policy, nil
}

func (s *Service) buildSkills(name string) ([]Skill, error) {
	entries, err := s.soil.List(catalogSkillsPrefix)
	if err != nil {
		return nil, err
	}
	skills := []Skill{}
	for _, entry := range entries {
		var sk skillEntry
		if json.Unmarshal(entry.Value, &sk) != nil {
			continue
		}
		if !assignedTo(sk.AssignedNims, name) {
			continue
		}
		skill := Skill{
			Name:         sk.Name,
			Description:  sk.Description,
			AgentType:    sk.AgentType,
			Path:         sk.Path,
			Excerpt:      sk.Excerpt,
			AssignedNims: sk.AssignedNims,
			Source:       SourceRef{Key: entry.Key, Revision: entry.Revision, Origin: OriginSoil},
		}
		contentKey := skillsContentPrefix + sk.AgentType + "." + sk.Name
		contentEntry, err := s.soil.Get(contentKey)
		switch {
		case err == nil:
			skill.Content = string(contentEntry.Value)
			skill.ContentSource = SourceRef{Key: contentKey, Revision: contentEntry.Revision, Origin: OriginSoil}
		case errors.Is(err, soil.ErrNotFound):
			// Description-only skill, same fallback the runtime uses.
			skill.ContentSource = SourceRef{Key: contentKey}
		default:
			return nil, err
		}
		skills = append(skills, skill)
	}
	return skills, nil
}

func (s *Service) assembleFromRegistry(name string) (*RoleBundle, error) {
	nim, err := s.registry.GetNim(name)
	if err != nil {
		if errors.Is(err, ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("%w: soil disabled and registry fallback failed: %v", ErrUnavailable, err)
	}
	identitySource := SourceRef{Key: catalogNimsPrefix + name, Revision: 0, Origin: OriginRegistry}
	identity := Identity{
		Name:            name,
		Role:            nim.Role,
		Description:     nim.Description,
		Category:        nim.Category,
		LongDescription: nim.LongDescription,
		Subjects:        nim.Subjects,
		Source:          identitySource,
		PromptSource:    SourceRef{Key: promptsNimsPrefix + name, Origin: OriginRegistry},
		// Ceremony prompts and backing humans live only in Soil, so
		// a degraded bundle says so instead of guessing.
		BackingSource: SourceRef{Key: agenthumanConfigKey},
	}
	if prompt, err := s.registry.GetPrompt(name); err == nil {
		identity.Prompt = prompt
	}

	return &RoleBundle{
		Name:        name,
		GeneratedAt: time.Now().UTC(),
		Identity:    identity,
		// Tool and skill assignments live only in Soil. Degraded
		// bundles say so instead of guessing.
		Policy: Policy{
			AgentType:    DefaultAgentType,
			Tools:        []ToolAssignment{},
			ConfigSource: SourceRef{Key: configNimsPrefix + name, Origin: OriginRegistry},
		},
		Skills:     []Skill{},
		Memory:     Memory{Namespace: memoryNamespace(name)},
		Docs:       s.buildDocs(name),
		Acceptance: BuildAcceptance(nim.LongDescription, identitySource),
		Degraded:   true,
	}, nil
}

// memoryNamespace is the declared per-role memory namespace. The
// store is out of scope; the declaration matches the per-nim
// conversation key space in Soil.
func memoryNamespace(name string) string {
	return "conversations." + name
}

// buildDocs is slot 5, the runbook and doc index. Every link is a
// relative path on the portal's own authenticated routes, so the
// index works for out-of-forest consumers; an internal service URL
// (for example the nimregistry loopback) would be dead off the land.
// Links only today. catalog.docs.* keys adopt this slot when issue
// #92 lands; until then there is no Soil source and the per-role
// runbook links stay pending (recorded in the portal runbook).
func (s *Service) buildDocs(name string) Docs {
	return Docs{
		Links: []DocLink{
			{Title: "Role page (markdown)", URL: "/roles/" + name + ".md"},
			{Title: "Role bundle (JSON)", URL: "/api/v1/roles/" + name + "/bundle"},
			{Title: "Role changelog", URL: "/roles/" + name + "/changelog"},
			{Title: "Shared doctrine (runbook culture, operating rules)", URL: "/doctrine"},
		},
		Source: SourceRef{Key: "", Revision: 0},
	}
}

func assignedTo(assigned []string, name string) bool {
	for _, a := range assigned {
		if a == "*" || a == name {
			return true
		}
	}
	return false
}

// Invalidate drops the cached bundle for one role and the cached
// role list.
func (s *Service) Invalidate(name string) {
	s.mu.Lock()
	s.gen++
	delete(s.bundles, name)
	s.list = nil
	s.listValid = false
	s.mu.Unlock()
}

// InvalidateAll drops the whole render cache.
func (s *Service) InvalidateAll() {
	s.mu.Lock()
	s.gen++
	s.bundles = make(map[string]*RoleBundle)
	s.list = nil
	s.listValid = false
	s.mu.Unlock()
}

// DisableCache turns the render cache off for the life of the
// service: every request reads through to Soil. Call it when the
// Soil watches could not start, so no stale render can be served
// silently.
func (s *Service) DisableCache() {
	s.mu.Lock()
	s.cacheDisabled = true
	s.bundles = make(map[string]*RoleBundle)
	s.list = nil
	s.listValid = false
	s.mu.Unlock()
}

// StartWatch subscribes the render cache to Soil changes, the
// anti-drift mechanism: a nim added through the catalog appears, a
// deleted nim disappears, and no stale render survives a change.
// The returned function stops all watches.
func (s *Service) StartWatch(w soil.Watcher) (func(), error) {
	patterns := []string{
		"catalog.>",
		"prompts.nims.>",
		"config.nims.>",
		"config.forest.>",
		"skills.content.>",
		"agenthuman.>",
	}
	var stops []func()
	stopAll := func() {
		for _, stop := range stops {
			stop()
		}
	}
	for _, pattern := range patterns {
		stop, err := w.Watch(pattern, s.invalidateForKey)
		if err != nil {
			stopAll()
			return nil, err
		}
		stops = append(stops, stop)
	}
	return stopAll, nil
}

// invalidateForKey maps a changed Soil key to the cache entries it
// touches. Per-nim keys invalidate that role; shared catalogs (tools,
// skills) can touch every bundle, so they clear the whole cache.
func (s *Service) invalidateForKey(key string) {
	switch {
	case strings.HasPrefix(key, catalogNimsPrefix):
		s.Invalidate(strings.TrimPrefix(key, catalogNimsPrefix))
	case strings.HasPrefix(key, promptsNimsPrefix):
		rest := strings.TrimPrefix(key, promptsNimsPrefix)
		// Ceremony subkeys (prompts.nims.<name>.audit and friends)
		// belong to the same role.
		if idx := strings.Index(rest, "."); idx > 0 {
			rest = rest[:idx]
		}
		s.Invalidate(rest)
	case strings.HasPrefix(key, configNimsPrefix):
		s.Invalidate(strings.TrimPrefix(key, configNimsPrefix))
	default:
		s.InvalidateAll()
	}
}
