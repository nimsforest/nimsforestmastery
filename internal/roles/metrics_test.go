package roles

import (
	"strings"
	"testing"
)

func TestParseLabelFormWithIntroAndNumberedItems(t *testing.T) {
	body := strings.Join([]string{
		"# Neo",
		"",
		"Some responsibilities text.",
		"",
		"Performance Metrics:",
		"",
		"To evaluate Neo's effectiveness, the following KPIs will be tracked:",
		"",
		"1. Development Velocity: Sprint completion rate and story points delivered.",
		"2. Code Quality: Code coverage by tests, number of bugs reported in production.",
		"",
		"---",
		"",
		"Trailing text after the rule.",
	}, "\n")

	section := ParseMetricsSection(body)
	if !section.Found {
		t.Fatal("section not found")
	}
	if !section.Parsed {
		t.Fatal("section not parsed")
	}
	if len(section.Metrics) != 2 {
		t.Fatalf("got %d metrics, want 2", len(section.Metrics))
	}
	if section.Metrics[0].Name != "Development Velocity" {
		t.Errorf("metric 0 name = %q", section.Metrics[0].Name)
	}
	if section.Metrics[0].Description != "Sprint completion rate and story points delivered." {
		t.Errorf("metric 0 description = %q", section.Metrics[0].Description)
	}
	if section.Metrics[1].Name != "Code Quality" {
		t.Errorf("metric 1 name = %q", section.Metrics[1].Name)
	}
	if strings.Contains(section.Raw, "Trailing text") {
		t.Error("raw section includes text after the horizontal rule")
	}
	if !strings.HasPrefix(section.Raw, "Performance Metrics:") {
		t.Errorf("raw section does not start with the label line: %q", section.Raw)
	}
}

func TestParseHeadingFormBareNumberedItems(t *testing.T) {
	body := strings.Join([]string{
		"## Performance Metrics",
		"",
		"1. Defects caught before production vs. defects found in production",
		"2. Review turnaround time",
		"",
		"## Collaborations",
		"",
		"Works with neo.",
	}, "\n")

	section := ParseMetricsSection(body)
	if !section.Found || !section.Parsed {
		t.Fatalf("found=%v parsed=%v, want both true", section.Found, section.Parsed)
	}
	if len(section.Metrics) != 2 {
		t.Fatalf("got %d metrics, want 2", len(section.Metrics))
	}
	if section.Metrics[0].Name != "" {
		t.Errorf("bare item has name %q, want none", section.Metrics[0].Name)
	}
	if section.Metrics[1].Description != "Review turnaround time" {
		t.Errorf("metric 1 description = %q", section.Metrics[1].Description)
	}
	if strings.Contains(section.Raw, "Collaborations") {
		t.Error("raw section crossed the next heading")
	}
}

func TestParseNumberedItemsWithDetail(t *testing.T) {
	body := strings.Join([]string{
		"## Performance Metrics",
		"",
		"1. Uptime: Percentage of time the service answers.",
		"- measured monthly",
		"   over rolling windows",
		"2. Latency",
	}, "\n")

	section := ParseMetricsSection(body)
	if len(section.Metrics) != 2 {
		t.Fatalf("got %d metrics, want 2", len(section.Metrics))
	}
	want := "Percentage of time the service answers. measured monthly over rolling windows"
	if section.Metrics[0].Description != want {
		t.Errorf("metric 0 description = %q, want %q", section.Metrics[0].Description, want)
	}
}

func TestParseDashItems(t *testing.T) {
	body := strings.Join([]string{
		"## Performance Metrics",
		"",
		"- Response time: under one day",
		"- Resolution rate",
	}, "\n")

	section := ParseMetricsSection(body)
	if len(section.Metrics) != 2 {
		t.Fatalf("got %d metrics, want 2", len(section.Metrics))
	}
	if section.Metrics[0].Name != "Response time" || section.Metrics[0].Description != "under one day" {
		t.Errorf("metric 0 = %+v", section.Metrics[0])
	}
	if section.Metrics[1].Description != "Resolution rate" {
		t.Errorf("metric 1 = %+v", section.Metrics[1])
	}
}

func TestParseSubheadingItems(t *testing.T) {
	body := strings.Join([]string{
		"## Performance Metrics",
		"",
		"### User Satisfaction",
		"Regular surveys and feedback analysis from NIMs using Nebula's products.",
		"",
		"### Adoption",
		"Share of NIMs that use the product weekly.",
	}, "\n")

	section := ParseMetricsSection(body)
	if len(section.Metrics) != 2 {
		t.Fatalf("got %d metrics, want 2", len(section.Metrics))
	}
	if section.Metrics[0].Name != "User Satisfaction" {
		t.Errorf("metric 0 name = %q", section.Metrics[0].Name)
	}
	if !strings.Contains(section.Metrics[0].Description, "Regular surveys") {
		t.Errorf("metric 0 description = %q", section.Metrics[0].Description)
	}
	if section.Metrics[1].Name != "Adoption" {
		t.Errorf("metric 1 name = %q", section.Metrics[1].Name)
	}
}

func TestParseParagraphItems(t *testing.T) {
	body := strings.Join([]string{
		"## Performance Metrics",
		"",
		"The service keeps its consumers informed within one business day",
		"of any change.",
		"",
		"Every escalation receives an answer.",
	}, "\n")

	section := ParseMetricsSection(body)
	if len(section.Metrics) != 2 {
		t.Fatalf("got %d metrics, want 2", len(section.Metrics))
	}
	want := "The service keeps its consumers informed within one business day of any change."
	if section.Metrics[0].Description != want {
		t.Errorf("metric 0 description = %q", section.Metrics[0].Description)
	}
}

func TestSectionEndsAtNewLabel(t *testing.T) {
	body := strings.Join([]string{
		"Performance Metrics:",
		"",
		"1. Throughput: items per week.",
		"",
		"Collaborations:",
		"",
		"1. Works with neo.",
	}, "\n")

	section := ParseMetricsSection(body)
	if len(section.Metrics) != 1 {
		t.Fatalf("got %d metrics, want 1", len(section.Metrics))
	}
	if strings.Contains(section.Raw, "Collaborations") {
		t.Error("raw section crossed the new label")
	}
}

func TestIntroOnlySectionFallsBackToVerbatim(t *testing.T) {
	body := strings.Join([]string{
		"## Performance Metrics",
		"",
		"The following KPIs will be tracked:",
		"",
	}, "\n")

	section := ParseMetricsSection(body)
	if !section.Found {
		t.Fatal("section not found")
	}
	if section.Parsed {
		t.Fatal("intro-only section must not count as parsed")
	}
	if !strings.Contains(section.Raw, "KPIs will be tracked") {
		t.Errorf("raw section = %q", section.Raw)
	}
}

func TestMissingSection(t *testing.T) {
	section := ParseMetricsSection("# A nim\n\nNo metrics here.\n")
	if section.Found {
		t.Fatal("section reported found in a body without one")
	}
}

func TestSectionMatchIsCaseSensitive(t *testing.T) {
	section := ParseMetricsSection("## performance metrics\n\n1. A thing\n")
	if section.Found {
		t.Fatal("lowercase heading must not match")
	}
}

func TestBuildAcceptanceStates(t *testing.T) {
	source := SourceRef{Key: "catalog.nims.neo", Revision: 7, Origin: OriginSoil}

	parsed := BuildAcceptance("## Performance Metrics\n\n1. Uptime: high.\n", source)
	if parsed.State != AcceptanceParsed {
		t.Errorf("state = %q, want %q", parsed.State, AcceptanceParsed)
	}
	if len(parsed.Criteria) != 1 || parsed.Criteria[0].Name != "Uptime" {
		t.Errorf("criteria = %+v", parsed.Criteria)
	}
	if parsed.Source.Revision != 7 {
		t.Errorf("source revision = %d, want 7", parsed.Source.Revision)
	}
	if parsed.Verbatim == "" {
		t.Error("parsed acceptance must keep the verbatim section")
	}

	verbatim := BuildAcceptance("## Performance Metrics\n\nWill be defined later:\n", source)
	if verbatim.State != AcceptanceVerbatim {
		t.Errorf("state = %q, want %q", verbatim.State, AcceptanceVerbatim)
	}
	if verbatim.Verbatim == "" {
		t.Error("verbatim acceptance carries no section text")
	}

	missing := BuildAcceptance("No section at all.", source)
	if missing.State != AcceptanceNotDefined {
		t.Errorf("state = %q, want %q", missing.State, AcceptanceNotDefined)
	}
	if missing.Message != NotDefinedMessage {
		t.Errorf("message = %q, want %q", missing.Message, NotDefinedMessage)
	}
}
