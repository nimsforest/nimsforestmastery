package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/nimsforest/nimsforestmastery/internal/roles"
)

// RenderRoleMarkdown renders one role bundle as a single plain
// markdown document: identity, behavior prompt, policy with tools,
// skills with full content, the docs index, the memory namespace, and
// the acceptance manifest. This document is the primary agent
// product; a Claude session is pointed straight at it.
func RenderRoleMarkdown(bundle *roles.RoleBundle, org string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Role: %s\n\n", bundle.Name)
	if bundle.Identity.Role != "" {
		fmt.Fprintf(&sb, "%s in the %s forest.\n", bundle.Identity.Role, org)
	} else {
		fmt.Fprintf(&sb, "A role in the %s forest.\n", org)
	}
	fmt.Fprintf(&sb, "Generated live from the catalog at %s.\n", bundle.GeneratedAt.Format(time.RFC3339))
	sb.WriteString("Do not edit this document. The source of truth is the forest's Soil;\n")
	sb.WriteString("each section names the key and revision it was generated from.\n")

	if bundle.Degraded {
		sb.WriteString("\nNOTE: Soil was not reachable. This document came from the\n")
		sb.WriteString("nimregistry fallback. Tool and skill assignments, ceremony\n")
		sb.WriteString("prompts and backing humans live only in Soil, so those\n")
		sb.WriteString("sections are empty here, not zero by design.\n")
	}

	// Slot 1: identity.
	sb.WriteString("\n## Identity\n\n")
	fmt.Fprintf(&sb, "- Name: %s\n", bundle.Identity.Name)
	if bundle.Identity.Role != "" {
		fmt.Fprintf(&sb, "- Role: %s\n", bundle.Identity.Role)
	}
	if bundle.Identity.Category != "" {
		fmt.Fprintf(&sb, "- Category: %s\n", bundle.Identity.Category)
	}
	if bundle.Identity.Description != "" {
		fmt.Fprintf(&sb, "- Description: %s\n", bundle.Identity.Description)
	}
	if len(bundle.Identity.Subjects) > 0 {
		fmt.Fprintf(&sb, "- Subjects: %s\n", strings.Join(bundle.Identity.Subjects, ", "))
	}
	if len(bundle.Identity.BackingHumans) > 0 {
		fmt.Fprintf(&sb, "- Backing humans: %s (%s)\n",
			strings.Join(bundle.Identity.BackingHumans, ", "),
			formatSource(bundle.Identity.BackingSource))
	}
	fmt.Fprintf(&sb, "- Source: %s\n", formatSource(bundle.Identity.Source))
	if bundle.Identity.LongDescription != "" {
		sb.WriteString("\n" + strings.TrimSpace(bundle.Identity.LongDescription) + "\n")
	}

	// Slot 1, behavior: the raw prompt markdown plus the ceremony
	// subkeys (audit, awaken, autonomate.*, eos), each stamped.
	sb.WriteString("\n## Behavior Prompt\n\n")
	if bundle.Identity.Prompt != "" {
		fmt.Fprintf(&sb, "Source: %s\n\n", formatSource(bundle.Identity.PromptSource))
		sb.WriteString(strings.TrimSpace(bundle.Identity.Prompt) + "\n")
	} else {
		sb.WriteString("This role has no behavior prompt in the catalog.\n")
	}

	for _, ceremony := range bundle.Identity.CeremonyPrompts {
		fmt.Fprintf(&sb, "\n### Ceremony: %s\n\n", ceremony.Name)
		fmt.Fprintf(&sb, "Source: %s\n\n", formatSource(ceremony.Source))
		sb.WriteString(strings.TrimSpace(ceremony.Content) + "\n")
	}

	// Slot 2: policy.
	sb.WriteString("\n## Policy\n\n")
	fmt.Fprintf(&sb, "- Agent type: %s\n", bundle.Policy.AgentType)
	if bundle.Policy.Model != "" {
		fmt.Fprintf(&sb, "- Model: %s\n", bundle.Policy.Model)
	}
	if bundle.Policy.Provider != "" {
		fmt.Fprintf(&sb, "- Provider: %s\n", bundle.Policy.Provider)
	}
	fmt.Fprintf(&sb, "- Source: %s\n", formatSource(bundle.Policy.ConfigSource))

	sb.WriteString("\n### Tools\n\n")
	if len(bundle.Policy.Tools) == 0 {
		sb.WriteString("No tools are assigned to this role.\n")
	}
	for _, tool := range bundle.Policy.Tools {
		fmt.Fprintf(&sb, "- %s: %s (%s)\n", tool.Name, tool.Description, formatSource(tool.Source))
	}

	// Slot 3: skills, split per audience. This document serves an
	// agent, so the agent-archetype skills carry full content; the
	// human-archetype drill variants are listed by name only.
	sb.WriteString("\n## Skills\n\n")
	agentSkills := bundle.AgentSkills()
	humanSkills := bundle.HumanSkills()
	if len(bundle.Skills) == 0 {
		sb.WriteString("No skills are assigned to this role.\n")
	} else if len(agentSkills) == 0 {
		sb.WriteString("No agent-archetype skills are assigned to this role.\n")
	}
	for _, skill := range agentSkills {
		fmt.Fprintf(&sb, "### Skill: %s (%s)\n\n", skill.Name, skill.AgentType)
		if skill.Description != "" {
			sb.WriteString(skill.Description + "\n")
		}
		fmt.Fprintf(&sb, "- Source: %s\n", formatSource(skill.Source))
		fmt.Fprintf(&sb, "- Content: %s\n", formatSource(skill.ContentSource))
		if skill.Content != "" {
			sb.WriteString("\n" + strings.TrimSpace(skill.Content) + "\n")
		} else {
			sb.WriteString("\nThe catalog has no full content for this skill; the\n")
			sb.WriteString("description above is the whole skill.\n")
		}
		sb.WriteString("\n")
	}
	if len(humanSkills) > 0 {
		sb.WriteString("### Human archetype skills\n\n")
		sb.WriteString("These drill variants serve the human Learn surface, not an\n")
		sb.WriteString("agent session:\n\n")
		for _, skill := range humanSkills {
			fmt.Fprintf(&sb, "- %s: %s (%s)\n", skill.Name, skill.Description, formatSource(skill.Source))
		}
		sb.WriteString("\n")
	}

	// Slot 5: the docs index.
	sb.WriteString("## Docs\n\n")
	if len(bundle.Docs.Links) == 0 {
		sb.WriteString("No docs are indexed for this role.\n")
	}
	for _, link := range bundle.Docs.Links {
		fmt.Fprintf(&sb, "- [%s](%s)\n", link.Title, link.URL)
	}

	// Slot 4: the memory namespace declaration.
	sb.WriteString("\n## Memory\n\n")
	fmt.Fprintf(&sb, "Namespace: %s. The store itself is outside this portal.\n", bundle.Memory.Namespace)

	// Slot 6: the acceptance manifest.
	sb.WriteString("\n## Acceptance Manifest\n\n")
	switch bundle.Acceptance.State {
	case roles.AcceptanceParsed:
		sb.WriteString("Each criterion below comes from one Performance Metric in the\n")
		sb.WriteString("role's intent.\n\n")
		for i, criterion := range bundle.Acceptance.Criteria {
			if criterion.Name != "" && criterion.Description != "" {
				fmt.Fprintf(&sb, "%d. %s: %s\n", i+1, criterion.Name, criterion.Description)
			} else if criterion.Name != "" {
				fmt.Fprintf(&sb, "%d. %s\n", i+1, criterion.Name)
			} else {
				fmt.Fprintf(&sb, "%d. %s\n", i+1, criterion.Description)
			}
		}
	case roles.AcceptanceVerbatim:
		sb.WriteString("The Performance Metrics section did not parse into separate\n")
		sb.WriteString("criteria. It renders verbatim:\n\n")
		sb.WriteString(strings.TrimSpace(bundle.Acceptance.Verbatim) + "\n")
	default:
		sb.WriteString(bundle.Acceptance.Message + "\n")
	}
	fmt.Fprintf(&sb, "\nSource: %s\n", formatSource(bundle.Acceptance.Source))

	return sb.String()
}

// formatSource renders one SourceRef as "key@revision (origin)". A
// revision of 0 means the value did not come from Soil.
func formatSource(ref roles.SourceRef) string {
	if ref.Key == "" {
		return "none"
	}
	switch ref.Origin {
	case roles.OriginSoil:
		return fmt.Sprintf("%s@%d (soil)", ref.Key, ref.Revision)
	case roles.OriginRegistry:
		return ref.Key + " (nimregistry fallback, revision 0)"
	default:
		return ref.Key + " (not present in soil)"
	}
}
