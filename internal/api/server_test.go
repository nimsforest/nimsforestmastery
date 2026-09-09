package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimsforest/nimsforestmastery/internal/roles"
	"github.com/nimsforest/nimsforestmastery/internal/soil"
)

// authedGet performs one authenticated GET against a portal wired
// like production: auth wrap around the agent surface.
func authedGet(t *testing.T, portal http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+validPAT)
	rec := httptest.NewRecorder()
	portal.ServeHTTP(rec, req)
	return rec
}

func TestRolesListAndBundleRoutes(t *testing.T) {
	iamnim := stubIamnim(t, validPAT, []string{"acme"})
	portal := newPortal(iamnim.URL)

	rec := authedGet(t, portal, "/api/v1/roles")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Organization string              `json:"organization"`
		Degraded     bool                `json:"degraded"`
		Roles        []roles.RoleSummary `json:"roles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Organization != "acme" || list.Degraded {
		t.Errorf("list header = %+v", list)
	}
	if len(list.Roles) != 1 || list.Roles[0].Name != "neo" || list.Roles[0].Source.Revision != 11 {
		t.Errorf("roles = %+v", list.Roles)
	}

	rec = authedGet(t, portal, "/api/v1/roles/neo/bundle")
	if rec.Code != http.StatusOK {
		t.Fatalf("bundle status = %d, body %s", rec.Code, rec.Body.String())
	}
	var bundle roles.RoleBundle
	if err := json.Unmarshal(rec.Body.Bytes(), &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Identity.Source.Revision != 11 || bundle.Identity.PromptSource.Revision != 21 {
		t.Errorf("bundle identity stamps = %+v", bundle.Identity)
	}
	if bundle.Policy.AgentType != "opencode" || len(bundle.Policy.Tools) != 1 {
		t.Errorf("bundle policy = %+v", bundle.Policy)
	}

	rec = authedGet(t, portal, "/api/v1/roles/nobody/bundle")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown role status = %d, want 404", rec.Code)
	}
}

func TestRoleMarkdownDocument(t *testing.T) {
	iamnim := stubIamnim(t, validPAT, []string{"acme"})
	portal := newPortal(iamnim.URL)

	rec := authedGet(t, portal, "/roles/neo.md")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("Content-Type = %q", ct)
	}

	doc := rec.Body.String()
	// The whole path is one markdown document: identity, behavior,
	// skills with content, tools, docs, acceptance, all stamped.
	for _, want := range []string{
		"# Role: neo",
		"## Identity",
		"- Backing humans: cederik (agenthuman.config@71 (soil))",
		"catalog.nims.neo@11 (soil)",
		"## Behavior Prompt",
		"Be the developer. Ship working software.",
		"prompts.nims.neo@21 (soil)",
		"### Ceremony: audit",
		"Audit ceremony: check the work.",
		"prompts.nims.neo.audit@22 (soil)",
		"### Ceremony: autonomate.intent",
		"prompts.nims.neo.autonomate.intent@23 (soil)",
		"### Ceremony: eos",
		"prompts.nims.neo.eos@24 (soil)",
		"## Policy",
		"- Agent type: opencode",
		"- Bash: Execute shell commands (catalog.tools.bash@41 (soil))",
		"### Skill: create-issue (ai)",
		"Full create-issue skill content body.",
		"skills.content.ai.create-issue@61 (soil)",
		"### Human archetype skills",
		"- campaign-brief: Write a brief (catalog.skills.human.campaign-brief@52 (soil))",
		"## Docs",
		"/api/v1/roles/neo/bundle",
		"/roles/neo.md",
		"## Memory",
		"conversations.neo",
		"## Acceptance Manifest",
		"1. Development Velocity: Sprint completion rate.",
		"2. Code Quality: Coverage by tests.",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document is missing %q\n%s", want, doc)
		}
	}

	if rec := authedGet(t, portal, "/roles/nobody.md"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown role status = %d, want 404", rec.Code)
	}
	if rec := authedGet(t, portal, "/roles/neo"); rec.Code != http.StatusNotFound {
		t.Errorf("missing .md suffix status = %d, want 404", rec.Code)
	}
}

func TestCraftedRoleNamesNeverReachTheRegistry(t *testing.T) {
	// The reviewer repro: /roles/..%2F..%2Fadmin.md and
	// /roles/neo%3Fx=1.md decoded into the {page} path value and were
	// concatenated into the nimregistry request path. The strict role
	// name check must answer 404 and send nothing to the registry.
	iamnim := stubIamnim(t, validPAT, []string{"acme"})

	var requested []string
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.RequestURI())
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(registry.Close)

	service := roles.NewService(soil.Disabled{Reason: "no nats"}, roles.NewRegistryClient(registry.URL))
	portal := NewAuth(iamnim.URL, "acme").Wrap(NewServer(service, "acme", "test"))

	for _, path := range []string{
		"/roles/..%2F..%2Fadmin.md",
		"/roles/neo%3Fx=1.md",
		"/roles/neo%23frag.md",
		"/api/v1/roles/..%2F..%2Fadmin/bundle",
	} {
		rec := authedGet(t, portal, path)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404, body %s", path, rec.Code, rec.Body.String())
		}
	}
	if len(requested) != 0 {
		t.Errorf("crafted names reached the registry: %v", requested)
	}
}

func TestMarkdownNotDefinedAcceptance(t *testing.T) {
	fixture := fixtureSoil()
	fixture.put("catalog.nims.nudge", 12, `{"name":"nudge","role":"Marketing","description":"Growth","category":"Growth","long_description":"No metrics section here."}`)
	service := roles.NewService(fixture, roles.NewRegistryClient("http://127.0.0.1:1"))
	bundle, err := service.Bundle("nudge")
	if err != nil {
		t.Fatal(err)
	}
	doc := RenderRoleMarkdown(bundle, "acme")
	if !strings.Contains(doc, roles.NotDefinedMessage) {
		t.Errorf("document does not carry the honest empty state\n%s", doc)
	}
	if !strings.Contains(doc, "This role has no behavior prompt in the catalog.") {
		t.Errorf("document does not state the missing prompt\n%s", doc)
	}
}

func TestLLMsTxt(t *testing.T) {
	iamnim := stubIamnim(t, validPAT, []string{"acme"})
	portal := newPortal(iamnim.URL)

	rec := authedGet(t, portal, "/llms.txt")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# acme role mastery",
		"[neo](/roles/neo.md)",
		"/api/v1/roles",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("llms.txt is missing %q\n%s", want, body)
		}
	}
}

func TestRoleRoutesAnswer503WhenNoForest(t *testing.T) {
	// No NATS and no nimregistry: the honest 503 with a reason, on
	// every role route.
	iamnim := stubIamnim(t, validPAT, []string{"acme"})
	service := roles.NewService(soil.Disabled{Reason: "no nats"}, roles.NewRegistryClient("http://127.0.0.1:1"))
	portal := NewAuth(iamnim.URL, "acme").Wrap(NewServer(service, "acme", "test"))

	for _, path := range []string{"/api/v1/roles", "/api/v1/roles/neo/bundle", "/roles/neo.md", "/llms.txt"} {
		rec := authedGet(t, portal, path)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s status = %d, want 503, body %s", path, rec.Code, rec.Body.String())
			continue
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s body is not JSON: %s", path, rec.Body.String())
			continue
		}
		if body["error"] != "role catalog unavailable" || body["reason"] == "" {
			t.Errorf("%s body = %v", path, body)
		}
	}
}

func TestDegradedBundleStillServesThroughFallback(t *testing.T) {
	// Soil is down but nimregistry answers: the bundle serves, marked
	// degraded, with revision 0 stamps.
	iamnim := stubIamnim(t, validPAT, []string{"acme"})
	registryMux := http.NewServeMux()
	registryMux.HandleFunc("GET /api/nims/neo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"neo","role":"Software Development","description":"The developer","category":"Engineering"}`))
	})
	registryMux.HandleFunc("GET /api/nims/neo/prompt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Be the developer."))
	})
	registry := httptest.NewServer(registryMux)
	t.Cleanup(registry.Close)

	service := roles.NewService(soil.Disabled{Reason: "no nats"}, roles.NewRegistryClient(registry.URL))
	portal := NewAuth(iamnim.URL, "acme").Wrap(NewServer(service, "acme", "test"))

	rec := authedGet(t, portal, "/roles/neo.md")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	doc := rec.Body.String()
	if !strings.Contains(doc, "nimregistry fallback") {
		t.Errorf("degraded document does not name the fallback\n%s", doc)
	}
	if !strings.Contains(doc, "NOTE: Soil was not reachable.") {
		t.Errorf("degraded document carries no degraded notice\n%s", doc)
	}
}
