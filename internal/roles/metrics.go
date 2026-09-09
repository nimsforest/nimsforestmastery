package roles

import (
	"regexp"
	"strings"
)

// This file parses the Performance Metrics section of a nim intent
// body, per nimregistry docs/PERFORMANCE_METRICS_CONVENTION.md. The
// convention is descriptive and the parsing is brittle by design
// choice, so the fallback rule is load bearing: a section that does
// not parse renders verbatim and never fails the bundle.

// Metric is one parsed metric.
type Metric struct {
	Name        string
	Description string
}

// MetricsSection is the parse result for one intent body.
type MetricsSection struct {
	// Found reports whether the body contains the section at all.
	Found bool
	// Raw is the verbatim section text, start line included.
	Raw string
	// Parsed reports whether at least one metric was extracted.
	// When Found is true and Parsed is false, render Raw verbatim.
	Parsed bool
	// Metrics are the parsed metrics, in order.
	Metrics []Metric
}

var (
	numberedItem = regexp.MustCompile(`^[0-9]+\.\s+(.*)$`)
	dashItem     = regexp.MustCompile(`^-\s+(.*)$`)
	subheading   = regexp.MustCompile(`^###\s+(.*)$`)
)

// ParseMetricsSection finds and parses the Performance Metrics
// section in an intent body (the markdown after the frontmatter,
// delivered to the portal as long_description).
func ParseMetricsSection(body string) MetricsSection {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")

	start := -1
	for i, line := range lines {
		if line == "## Performance Metrics" || line == "Performance Metrics:" {
			start = i
			break
		}
	}
	if start < 0 {
		return MetricsSection{Found: false}
	}

	end := sectionEnd(lines, start)
	raw := strings.Join(lines[start:end], "\n")
	metrics := parseMetricsBody(lines[start+1 : end])

	return MetricsSection{
		Found:   true,
		Raw:     raw,
		Parsed:  len(metrics) > 0,
		Metrics: metrics,
	}
}

// sectionEnd scans forward from the start line and returns the index
// of the first line after the section.
func sectionEnd(lines []string, start int) int {
	metricContentSeen := false
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "## ") {
			return i
		}
		if trimmed == "---" {
			return i
		}
		if isLabelLine(line) && metricContentSeen {
			return i
		}
		if trimmed != "" && !strings.HasSuffix(trimmed, ":") {
			metricContentSeen = true
		}
	}
	return len(lines)
}

// isLabelLine reports whether a line is a new label: a column-0 line
// that ends with ":" and is not a numbered or dash item.
func isLabelLine(line string) bool {
	if line == "" || line != strings.TrimLeft(line, " \t") {
		return false
	}
	if !strings.HasSuffix(strings.TrimRight(line, " \t"), ":") {
		return false
	}
	return !numberedItem.MatchString(line) && !dashItem.MatchString(line)
}

// parseMetricsBody reads the section body: skip the intro, then try
// the item shapes in the fixed order, then fall back to paragraphs.
func parseMetricsBody(lines []string) []Metric {
	// Skip the optional intro: leading lines that end with ":".
	idx := 0
	for idx < len(lines) {
		trimmed := strings.TrimSpace(lines[idx])
		if trimmed == "" || strings.HasSuffix(trimmed, ":") {
			idx++
			continue
		}
		break
	}
	rest := lines[idx:]
	if len(rest) == 0 {
		return nil
	}

	switch {
	case anyMatch(rest, numberedItem):
		return parseListItems(rest, numberedItem, true)
	case anyMatch(rest, dashItem):
		return parseListItems(rest, dashItem, false)
	case anyMatch(rest, subheading):
		return parseSubheadingItems(rest)
	default:
		return parseParagraphItems(rest)
	}
}

func anyMatch(lines []string, re *regexp.Regexp) bool {
	for _, line := range lines {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// parseListItems parses numbered or dash items. For numbered items,
// dash bullets and indented prose under an item are detail that
// belongs to that item. For dash items, only indented lines are
// detail; a column-0 dash starts a new item.
func parseListItems(lines []string, item *regexp.Regexp, foldDashes bool) []Metric {
	var metrics []Metric
	current := -1
	for _, line := range lines {
		if m := item.FindStringSubmatch(line); m != nil {
			metrics = append(metrics, splitMetric(strings.TrimSpace(m[1])))
			current = len(metrics) - 1
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || current < 0 {
			continue
		}
		indented := line != strings.TrimLeft(line, " \t")
		detail := indented
		if foldDashes && strings.HasPrefix(line, "- ") {
			detail = true
		}
		if detail {
			// Strip the bullet marker from detail lines.
			text := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			metrics[current].Description = joinDetail(metrics[current].Description, text)
		} else {
			// Column-0 prose that is not an item belongs to no
			// metric; the verbatim render keeps it.
			current = -1
		}
	}
	return metrics
}

// parseSubheadingItems parses "### <name>" headings, each followed by
// a description paragraph.
func parseSubheadingItems(lines []string) []Metric {
	var metrics []Metric
	current := -1
	for _, line := range lines {
		if m := subheading.FindStringSubmatch(line); m != nil {
			metrics = append(metrics, Metric{Name: strings.TrimSpace(m[1])})
			current = len(metrics) - 1
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || current < 0 {
			continue
		}
		metrics[current].Description = joinDetail(metrics[current].Description, trimmed)
	}
	return metrics
}

// parseParagraphItems treats each block of non-blank lines as one
// metric.
func parseParagraphItems(lines []string) []Metric {
	var metrics []Metric
	var block []string
	flush := func() {
		if len(block) > 0 {
			metrics = append(metrics, splitMetric(strings.Join(block, " ")))
			block = nil
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		block = append(block, trimmed)
	}
	flush()
	return metrics
}

// splitMetric splits "<name>: <description>" at the first ": " into a
// metric name and its description. An item without that separator is
// a description with no separate name.
func splitMetric(text string) Metric {
	if idx := strings.Index(text, ": "); idx > 0 {
		return Metric{
			Name:        strings.TrimSpace(text[:idx]),
			Description: strings.TrimSpace(text[idx+2:]),
		}
	}
	if strings.HasSuffix(text, ":") && len(text) > 1 {
		return Metric{Name: strings.TrimSuffix(text, ":")}
	}
	return Metric{Description: text}
}

func joinDetail(existing, more string) string {
	if existing == "" {
		return more
	}
	return existing + " " + more
}

// BuildAcceptance derives the acceptance slot from an intent's
// long_description. It never fails: the three states are parsed,
// verbatim, and not_defined.
func BuildAcceptance(longDescription string, source SourceRef) Acceptance {
	section := ParseMetricsSection(longDescription)
	if !section.Found {
		return Acceptance{
			State:   AcceptanceNotDefined,
			Message: NotDefinedMessage,
			Source:  source,
		}
	}
	if !section.Parsed {
		return Acceptance{
			State:    AcceptanceVerbatim,
			Verbatim: section.Raw,
			Source:   source,
		}
	}
	criteria := make([]Criterion, 0, len(section.Metrics))
	for _, metric := range section.Metrics {
		criteria = append(criteria, Criterion{Name: metric.Name, Description: metric.Description})
	}
	return Acceptance{
		State:    AcceptanceParsed,
		Criteria: criteria,
		Verbatim: section.Raw,
		Source:   source,
	}
}
