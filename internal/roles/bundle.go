// Package roles generates role bundles from the live catalog. A role
// bundle is one nim rendered as a role a learner can take on. Nothing
// in this package is authored per nim: every slot is read from Soil,
// with the nimregistry HTTP API as the fallback read path.
package roles

import (
	"errors"
	"time"
)

// ErrRoleNotFound reports that no nim with the requested name exists
// in the catalog.
var ErrRoleNotFound = errors.New("roles: role not found")

// ErrUnavailable reports that neither Soil nor the nimregistry
// fallback could serve the request. The API layer renders it as an
// honest 503 with the reason, never as stale content.
var ErrUnavailable = errors.New("roles: catalog unavailable")

// Read-path names for SourceRef.Origin.
const (
	OriginSoil     = "soil"
	OriginRegistry = "nimregistry"
)

// SourceRef stamps one slot with the Soil key and revision it was
// generated from. Revision 0 means the value did not come from Soil:
// either the key was absent or the fallback read path served it.
// The stamps make a stale local bundle detectable.
type SourceRef struct {
	Key      string `json:"key"`
	Revision uint64 `json:"revision"`
	Origin   string `json:"origin,omitempty"`
}

// CeremonyPrompt is one ceremony subkey of the behavior slot:
// prompts.nims.<name>.audit, .awaken, .autonomate.*, .eos and any
// other subkey the runtime adds. The name is the subkey suffix after
// prompts.nims.<name>., for example "audit" or "autonomate.intent".
type CeremonyPrompt struct {
	Name    string    `json:"name"`
	Content string    `json:"content"`
	Source  SourceRef `json:"source"`
}

// Identity is slot 1: who the nim is, from catalog.nims.<name>, plus
// the behavior prompt from prompts.nims.<name> and its ceremony
// subkeys, plus the backing humans from agenthuman.config.
type Identity struct {
	Name            string    `json:"name"`
	Role            string    `json:"role"`
	Description     string    `json:"description"`
	Category        string    `json:"category"`
	LongDescription string    `json:"long_description,omitempty"`
	Subjects        []string  `json:"subjects,omitempty"`
	Source          SourceRef `json:"source"`

	// Prompt is the raw markdown of prompts.nims.<name>. Empty when
	// the nim has no prompt.
	Prompt       string    `json:"prompt,omitempty"`
	PromptSource SourceRef `json:"prompt_source"`

	// CeremonyPrompts are the prompts.nims.<name>.* subkeys, sorted
	// by key. Empty when the nim declares none, and always empty on
	// the fallback read path: ceremony prompts live only in Soil.
	CeremonyPrompts []CeremonyPrompt `json:"ceremony_prompts,omitempty"`

	// BackingHumans are the human names from agenthuman.config whose
	// backs list contains this nim, sorted. The Meet page shows who
	// already backs the role.
	BackingHumans []string  `json:"backing_humans,omitempty"`
	BackingSource SourceRef `json:"backing_source"`
}

// ToolAssignment is one tool granted to the nim through the
// nimsforesttools assignments (catalog.tools.*).
type ToolAssignment struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Source      SourceRef `json:"source"`
}

// Policy is slot 2: tools from nimsforesttools assignments, agent_type
// and model from config.nims.<name> as the runtime reads them today.
// Model routing (#283) is not built; bundles target agent_type.
type Policy struct {
	// AgentType is the effective backend. When config.nims.<name>
	// declares none, this is the runtime default "claudecode".
	AgentType string `json:"agent_type"`
	Model     string `json:"model,omitempty"`
	Provider  string `json:"provider,omitempty"`

	Tools        []ToolAssignment `json:"tools"`
	ConfigSource SourceRef        `json:"config_source"`
}

// DefaultAgentType matches the runtime fallback in
// nimsforest2 pkg/runtime/agentroutertreehouse.go.
const DefaultAgentType = "claudecode"

// Skill is one assigned skill from catalog.skills.<agent_type>.<name>
// with its full content from skills.content.<agent_type>.<name>.
type Skill struct {
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	AgentType    string    `json:"agent_type"`
	Path         string    `json:"path,omitempty"`
	Excerpt      string    `json:"excerpt,omitempty"`
	AssignedNims []string  `json:"assigned_nims,omitempty"`
	Source       SourceRef `json:"source"`

	// Content is the full SKILL.md. Empty when the content key is
	// absent; the description then carries the skill alone.
	Content       string    `json:"content,omitempty"`
	ContentSource SourceRef `json:"content_source"`
}

// Memory is slot 4: a namespace declaration only. The store itself is
// out of scope for the portal.
type Memory struct {
	Namespace string `json:"namespace"`
}

// DocLink is one entry in the docs index.
type DocLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Docs is slot 5: a doc index. Links today; catalog.docs.* keys when
// issue #92 lands, behind this same shape.
type Docs struct {
	Links  []DocLink `json:"links"`
	Source SourceRef `json:"source"`
}

// Acceptance states.
const (
	// AcceptanceParsed: the Performance Metrics section parsed into
	// criteria.
	AcceptanceParsed = "parsed"
	// AcceptanceVerbatim: the section exists but did not parse; it
	// renders verbatim and the bundle does not fail.
	AcceptanceVerbatim = "verbatim"
	// AcceptanceNotDefined: the intent has no Performance Metrics
	// section. The path renders an honest empty state.
	AcceptanceNotDefined = "not_defined"
)

// NotDefinedMessage is the honest empty state text.
const NotDefinedMessage = "acceptance criteria not defined"

// Criterion is one acceptance criterion derived from one metric. The
// check type (script or graded) and the rubric live in the manifest
// layer, not here.
type Criterion struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description"`
}

// Acceptance is slot 6: the acceptance manifest derived from the
// intent's Performance Metrics section.
type Acceptance struct {
	State    string      `json:"state"`
	Criteria []Criterion `json:"criteria,omitempty"`
	// Verbatim carries the raw section text. It is set in the
	// verbatim state, and also alongside parsed criteria so a
	// consumer can always show the source.
	Verbatim string `json:"verbatim,omitempty"`
	// Message is set in the not_defined state.
	Message string    `json:"message,omitempty"`
	Source  SourceRef `json:"source"`
}

// RoleBundle is the six-slot bundle, generated at request time.
type RoleBundle struct {
	Name        string    `json:"name"`
	GeneratedAt time.Time `json:"generated_at"`

	Identity   Identity   `json:"identity"`
	Policy     Policy     `json:"policy"`
	Skills     []Skill    `json:"skills"`
	Memory     Memory     `json:"memory"`
	Docs       Docs       `json:"docs"`
	Acceptance Acceptance `json:"acceptance"`

	// Degraded is true when Soil was not readable and the bundle
	// came from the fallback read path. Tool and skill assignments
	// live only in Soil, so those slots are empty when degraded.
	Degraded bool `json:"degraded"`
}

// HumanArchetype is the skill agent_type that targets human learners.
// Every other agent_type targets agents.
const HumanArchetype = "human"

// AgentSkills returns the assigned skills whose archetype targets
// agents: everything except the human archetype. The agent surfaces
// materialize these.
func (b *RoleBundle) AgentSkills() []Skill {
	var out []Skill
	for _, s := range b.Skills {
		if s.AgentType != HumanArchetype {
			out = append(out, s)
		}
	}
	return out
}

// HumanSkills returns the assigned human-archetype skills. The Learn
// page renders these as full drill units for human learners.
func (b *RoleBundle) HumanSkills() []Skill {
	var out []Skill
	for _, s := range b.Skills {
		if s.AgentType == HumanArchetype {
			out = append(out, s)
		}
	}
	return out
}

// RoleSummary is one row of the live role directory.
type RoleSummary struct {
	Name        string    `json:"name"`
	Role        string    `json:"role"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Source      SourceRef `json:"source"`
}
