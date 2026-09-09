package roles

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/nimsforest/nimsforestmastery/internal/soil"
)

// fakeSoil implements soil.Reader over a fixture map.
type fakeSoil struct {
	entries map[string]soil.Entry
}

func newFakeSoil() *fakeSoil {
	return &fakeSoil{entries: make(map[string]soil.Entry)}
}

func (f *fakeSoil) put(key string, revision uint64, value string) {
	f.entries[key] = soil.Entry{Key: key, Value: []byte(value), Revision: revision}
}

func (f *fakeSoil) Get(key string) (soil.Entry, error) {
	entry, ok := f.entries[key]
	if !ok {
		return soil.Entry{}, soil.ErrNotFound
	}
	return entry, nil
}

func (f *fakeSoil) List(prefix string) ([]soil.Entry, error) {
	var entries []soil.Entry
	for key, entry := range f.entries {
		if strings.HasPrefix(key, prefix) {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries, nil
}

func (f *fakeSoil) Disabled() bool { return false }

// fakeWatcher implements soil.Watcher and lets a test fire changes.
type fakeWatcher struct {
	callbacks []func(string)
}

func (w *fakeWatcher) Watch(pattern string, onChange func(key string)) (func(), error) {
	w.callbacks = append(w.callbacks, onChange)
	return func() {}, nil
}

func (w *fakeWatcher) fire(key string) {
	for _, cb := range w.callbacks {
		cb(key)
	}
}

const neoLongDescription = "Neo builds the platform.\n\n" +
	"Performance Metrics:\n\n" +
	"To evaluate Neo's effectiveness, the following KPIs will be tracked:\n\n" +
	"1. Development Velocity: Sprint completion rate.\n" +
	"2. Code Quality: Coverage by tests.\n"

func fixtureSoil() *fakeSoil {
	f := newFakeSoil()
	neo, _ := json.Marshal(map[string]any{
		"name": "neo", "role": "Software Development", "description": "The developer",
		"category": "Engineering", "long_description": neoLongDescription,
		"subjects": []string{"message.neo"},
	})
	f.put("catalog.nims.neo", 11, string(neo))
	f.put("catalog.nims.nudge", 12, `{"name":"nudge","role":"Marketing","description":"Growth","category":"Growth","long_description":"No metrics section here."}`)
	f.put("prompts.nims.neo", 21, "# Neo prompt\n\nBe the developer.")
	f.put("prompts.nims.neo.audit", 22, "Audit ceremony body.")
	f.put("prompts.nims.neo.autonomate.intent", 23, "Autonomate intent body.")
	f.put("prompts.nims.neo.eos", 24, "EOS ceremony body.")
	f.put("agenthuman.config", 25, `{"humans":{"cederik":{"backs":["neo"]},"julie":{"backs":["nudge"]},"max":{"backs":["nectar","neo"]}}}`)
	f.put("config.nims.neo", 31, `{"agent_type":"opencode","model":"gpt-5","provider":"openai"}`)
	f.put("catalog.tools.bash", 41, `{"name":"Bash","description":"Execute shell commands","assigned_nims":["neo","numbers"]}`)
	f.put("catalog.tools.read", 42, `{"name":"Read","description":"Read files","assigned_nims":["*"]}`)
	f.put("catalog.tools.agent", 43, `{"name":"Agent","description":"Spawn agents","assigned_nims":["nightwatch"]}`)
	f.put("catalog.skills.ai.create-issue", 51, `{"name":"create-issue","description":"File an issue","agent_type":"ai","path":"agents/ai/skills/create-issue/SKILL.md","excerpt":"File...","assigned_nims":["neo"]}`)
	f.put("catalog.skills.human.campaign-brief", 52, `{"name":"campaign-brief","description":"Write a brief","agent_type":"human","path":"agents/human/skills/campaign-brief/SKILL.md","excerpt":"Write...","assigned_nims":["nudge"]}`)
	f.put("skills.content.ai.create-issue", 61, "# create-issue\n\nFull skill content.")
	// A non-nim catalog key that must never appear in the role list.
	f.put("catalog.mastery.doctrine.forest", 71, `{"title":"What a forest is"}`)
	return f
}

func newTestService(f *fakeSoil) *Service {
	return NewService(f, NewRegistryClient("http://127.0.0.1:8101"))
}

func TestListRolesEnumeratesOnlyCatalogNims(t *testing.T) {
	service := newTestService(fixtureSoil())
	list, err := service.ListRoles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d roles, want 2: %+v", len(list), list)
	}
	if list[0].Name != "neo" || list[1].Name != "nudge" {
		t.Errorf("role names = %q, %q", list[0].Name, list[1].Name)
	}
	if list[0].Source.Key != "catalog.nims.neo" || list[0].Source.Revision != 11 {
		t.Errorf("neo source = %+v", list[0].Source)
	}
	if list[0].Source.Origin != OriginSoil {
		t.Errorf("neo origin = %q", list[0].Source.Origin)
	}
}

func TestBundleAssemblyWithRevisionStamps(t *testing.T) {
	service := newTestService(fixtureSoil())
	bundle, err := service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}

	if bundle.Degraded {
		t.Error("bundle marked degraded on a healthy soil")
	}

	// Slot 1: identity plus prompt, each stamped.
	if bundle.Identity.Role != "Software Development" {
		t.Errorf("identity role = %q", bundle.Identity.Role)
	}
	if bundle.Identity.Source.Revision != 11 {
		t.Errorf("identity revision = %d, want 11", bundle.Identity.Source.Revision)
	}
	if bundle.Identity.Prompt == "" || bundle.Identity.PromptSource.Revision != 21 {
		t.Errorf("prompt source = %+v", bundle.Identity.PromptSource)
	}

	// Slot 1, behavior ceremonies: every prompts.nims.neo.* subkey is
	// in the bundle, sorted by key, each with its own revision stamp.
	ceremonies := bundle.Identity.CeremonyPrompts
	if len(ceremonies) != 3 {
		t.Fatalf("got %d ceremony prompts, want 3: %+v", len(ceremonies), ceremonies)
	}
	wantCeremonies := []struct {
		name     string
		key      string
		revision uint64
	}{
		{"audit", "prompts.nims.neo.audit", 22},
		{"autonomate.intent", "prompts.nims.neo.autonomate.intent", 23},
		{"eos", "prompts.nims.neo.eos", 24},
	}
	for i, want := range wantCeremonies {
		got := ceremonies[i]
		if got.Name != want.name || got.Source.Key != want.key || got.Source.Revision != want.revision || got.Content == "" {
			t.Errorf("ceremony[%d] = %+v, want %+v", i, got, want)
		}
	}

	// Slot 1, backing humans from agenthuman.config, exact nim match,
	// sorted, stamped.
	if len(bundle.Identity.BackingHumans) != 2 ||
		bundle.Identity.BackingHumans[0] != "cederik" || bundle.Identity.BackingHumans[1] != "max" {
		t.Errorf("backing humans = %+v", bundle.Identity.BackingHumans)
	}
	if bundle.Identity.BackingSource.Key != "agenthuman.config" || bundle.Identity.BackingSource.Revision != 25 {
		t.Errorf("backing source = %+v", bundle.Identity.BackingSource)
	}

	// Slot 2: policy from config.nims plus tool assignments.
	if bundle.Policy.AgentType != "opencode" || bundle.Policy.Model != "gpt-5" || bundle.Policy.Provider != "openai" {
		t.Errorf("policy = %+v", bundle.Policy)
	}
	if bundle.Policy.ConfigSource.Revision != 31 {
		t.Errorf("config revision = %d, want 31", bundle.Policy.ConfigSource.Revision)
	}
	toolNames := []string{}
	for _, tool := range bundle.Policy.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	if len(toolNames) != 2 || toolNames[0] != "Bash" || toolNames[1] != "Read" {
		t.Errorf("tools = %v, want [Bash Read]", toolNames)
	}
	if bundle.Policy.Tools[0].Source.Revision != 41 {
		t.Errorf("bash tool revision = %d, want 41", bundle.Policy.Tools[0].Source.Revision)
	}

	// Slot 3: skills filtered by assigned_nims, content resolved.
	if len(bundle.Skills) != 1 {
		t.Fatalf("got %d skills, want 1: %+v", len(bundle.Skills), bundle.Skills)
	}
	skill := bundle.Skills[0]
	if skill.Name != "create-issue" || skill.Source.Revision != 51 {
		t.Errorf("skill = %+v", skill)
	}
	if skill.Content == "" || skill.ContentSource.Key != "skills.content.ai.create-issue" || skill.ContentSource.Revision != 61 {
		t.Errorf("skill content source = %+v", skill.ContentSource)
	}

	// Slot 4: memory is a namespace declaration only.
	if bundle.Memory.Namespace != "conversations.neo" {
		t.Errorf("memory namespace = %q", bundle.Memory.Namespace)
	}

	// Slot 5: docs index carries links.
	if len(bundle.Docs.Links) == 0 {
		t.Error("docs index is empty")
	}

	// Slot 6: acceptance parsed from the Performance Metrics section,
	// stamped with the identity source revision.
	if bundle.Acceptance.State != AcceptanceParsed {
		t.Errorf("acceptance state = %q", bundle.Acceptance.State)
	}
	if len(bundle.Acceptance.Criteria) != 2 || bundle.Acceptance.Criteria[0].Name != "Development Velocity" {
		t.Errorf("criteria = %+v", bundle.Acceptance.Criteria)
	}
	if bundle.Acceptance.Source.Revision != 11 {
		t.Errorf("acceptance revision = %d, want 11", bundle.Acceptance.Source.Revision)
	}
}

func TestBundleDefaultsAndHonestStates(t *testing.T) {
	service := newTestService(fixtureSoil())
	bundle, err := service.Bundle("nudge")
	if err != nil {
		t.Fatal(err)
	}
	// No config.nims.nudge: the runtime default applies, revision 0.
	if bundle.Policy.AgentType != DefaultAgentType {
		t.Errorf("agent type = %q, want %q", bundle.Policy.AgentType, DefaultAgentType)
	}
	if bundle.Policy.ConfigSource.Revision != 0 {
		t.Errorf("config revision = %d, want 0", bundle.Policy.ConfigSource.Revision)
	}
	// No prompt: empty with revision 0.
	if bundle.Identity.Prompt != "" || bundle.Identity.PromptSource.Revision != 0 {
		t.Errorf("prompt source = %+v", bundle.Identity.PromptSource)
	}
	// campaign-brief is assigned to nudge; it has no content key.
	if len(bundle.Skills) != 1 || bundle.Skills[0].Name != "campaign-brief" {
		t.Fatalf("skills = %+v", bundle.Skills)
	}
	if bundle.Skills[0].Content != "" || bundle.Skills[0].ContentSource.Revision != 0 {
		t.Errorf("content source = %+v", bundle.Skills[0].ContentSource)
	}
	// No Performance Metrics section: the honest empty state.
	if bundle.Acceptance.State != AcceptanceNotDefined || bundle.Acceptance.Message != NotDefinedMessage {
		t.Errorf("acceptance = %+v", bundle.Acceptance)
	}
}

func TestBundleUnknownRole(t *testing.T) {
	service := newTestService(fixtureSoil())
	if _, err := service.Bundle("nobody"); err != ErrRoleNotFound {
		t.Fatalf("err = %v, want ErrRoleNotFound", err)
	}
}

func TestBundleRejectsInvalidRoleNames(t *testing.T) {
	// A crafted role name must never reach a Soil key or a registry
	// request path: it is not found, full stop.
	service := newTestService(fixtureSoil())
	for _, name := range []string{
		"", "../admin", "..%2F..%2Fadmin", "neo?x=1", "neo#frag",
		"neo/prompt", "NEO", "neo neo", "..", ".neo",
	} {
		if _, err := service.Bundle(name); err != ErrRoleNotFound {
			t.Errorf("Bundle(%q) err = %v, want ErrRoleNotFound", name, err)
		}
	}
	for _, name := range []string{"neo", "nudge", "growth-lead", "a2", "role_x", "v1.role"} {
		if !ValidRoleName(name) {
			t.Errorf("ValidRoleName(%q) = false, want true", name)
		}
	}
}

func TestOrgDefaultAgentTypeMiddleLayer(t *testing.T) {
	// The runtime fallback chain is per-nim config, then the org
	// default config.forest.agent_type, then "claudecode".
	fixture := fixtureSoil()
	fixture.put("config.forest.agent_type", 91, `"opencode"`)
	service := newTestService(fixture)

	// nudge has no per-nim config: the org default applies.
	bundle, err := service.Bundle("nudge")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Policy.AgentType != "opencode" {
		t.Errorf("agent type = %q, want the org default opencode", bundle.Policy.AgentType)
	}

	// neo has per-nim config: it wins over the org default.
	fixture.put("config.forest.agent_type", 92, "otherbackend")
	service = newTestService(fixture)
	bundle, err = service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Policy.AgentType != "opencode" {
		t.Errorf("agent type = %q, want the per-nim opencode", bundle.Policy.AgentType)
	}

	// The legacy "ai" value means no override, like the runtime.
	fixture.put("config.forest.agent_type", 93, "ai")
	service = newTestService(fixture)
	bundle, err = service.Bundle("nudge")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Policy.AgentType != DefaultAgentType {
		t.Errorf("agent type = %q, want %q", bundle.Policy.AgentType, DefaultAgentType)
	}
}

func TestSkillAudienceSplit(t *testing.T) {
	fixture := fixtureSoil()
	fixture.put("catalog.skills.human.pairing", 55, `{"name":"pairing","description":"Pair on a task","agent_type":"human","assigned_nims":["neo"]}`)
	service := newTestService(fixture)
	bundle, err := service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}
	agentSkills := bundle.AgentSkills()
	humanSkills := bundle.HumanSkills()
	if len(agentSkills) != 1 || agentSkills[0].Name != "create-issue" {
		t.Errorf("agent skills = %+v", agentSkills)
	}
	if len(humanSkills) != 1 || humanSkills[0].Name != "pairing" {
		t.Errorf("human skills = %+v", humanSkills)
	}
}

func TestDocsLinksAreRelativePortalRoutes(t *testing.T) {
	// The docs index serves out-of-forest consumers, so an internal
	// service URL (the nimregistry loopback) must never appear.
	service := newTestService(fixtureSoil())
	bundle, err := service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Docs.Links) == 0 {
		t.Fatal("docs index is empty")
	}
	for _, link := range bundle.Docs.Links {
		if !strings.HasPrefix(link.URL, "/") || strings.Contains(link.URL, "127.0.0.1") {
			t.Errorf("doc link %q is not a relative portal route", link.URL)
		}
	}
}

func TestDisableCacheServesReadThrough(t *testing.T) {
	fixture := fixtureSoil()
	service := newTestService(fixture)
	service.DisableCache()

	first, err := service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}
	// Change the key with no watch running: the next read must see it.
	neo, _ := json.Marshal(map[string]any{
		"name": "neo", "role": "Renamed Role", "description": "The developer",
		"category": "Engineering", "long_description": neoLongDescription,
	})
	fixture.put("catalog.nims.neo", 99, string(neo))
	fresh, err := service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Identity.Source.Revision == first.Identity.Source.Revision {
		t.Error("disabled cache still served a cached bundle")
	}

	if _, err := service.ListRoles(); err != nil {
		t.Fatal(err)
	}
	fixture.put("catalog.nims.notary", 80, `{"name":"notary","role":"Records","description":"Keeps records","category":"Operations"}`)
	list, err := service.ListRoles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Errorf("disabled cache still served a cached list: %d roles", len(list))
	}
}

// racingSoil fires an invalidation between the Soil reads of one
// assemble and its cache insert, the TOCTOU window.
type racingSoil struct {
	*fakeSoil
	service *Service
	role    string
	once    sync.Once
}

func (r *racingSoil) Get(key string) (soil.Entry, error) {
	entry, err := r.fakeSoil.Get(key)
	if key == "catalog.nims."+r.role {
		r.once.Do(func() {
			// The watched key changes while assemble is in flight.
			r.service.Invalidate(r.role)
		})
	}
	return entry, err
}

func TestInvalidationDuringAssembleIsNotLost(t *testing.T) {
	fixture := fixtureSoil()
	racing := &racingSoil{fakeSoil: fixture, role: "neo"}
	service := NewService(racing, NewRegistryClient("http://127.0.0.1:8101"))
	racing.service = service

	if _, err := service.Bundle("neo"); err != nil {
		t.Fatal(err)
	}

	// The invalidation fired mid-assemble, so the result must not
	// have entered the cache: a later read sees the new revision.
	neo, _ := json.Marshal(map[string]any{
		"name": "neo", "role": "Renamed Role", "description": "The developer",
		"category": "Engineering", "long_description": neoLongDescription,
	})
	fixture.put("catalog.nims.neo", 99, string(neo))
	fresh, err := service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Identity.Source.Revision != 99 {
		t.Errorf("stale bundle survived a mid-assemble invalidation: %+v", fresh.Identity.Source)
	}
}

func TestWatchOnCeremonySubkeyInvalidatesRole(t *testing.T) {
	fixture := fixtureSoil()
	service := newTestService(fixture)
	watcher := &fakeWatcher{}
	stop, err := service.StartWatch(watcher)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	if _, err := service.Bundle("neo"); err != nil {
		t.Fatal(err)
	}
	fixture.put("prompts.nims.neo.audit", 90, "Updated audit ceremony.")
	watcher.fire("prompts.nims.neo.audit")

	fresh, err := service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ceremony := range fresh.Identity.CeremonyPrompts {
		if ceremony.Name == "audit" && ceremony.Source.Revision == 90 {
			found = true
		}
	}
	if !found {
		t.Errorf("ceremony change did not invalidate the bundle: %+v", fresh.Identity.CeremonyPrompts)
	}
}

func TestWatchInvalidatesRenderCache(t *testing.T) {
	fixture := fixtureSoil()
	service := newTestService(fixture)
	watcher := &fakeWatcher{}
	stop, err := service.StartWatch(watcher)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	first, err := service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}

	// Change the underlying key. Without invalidation the cache
	// still serves the old render.
	neo, _ := json.Marshal(map[string]any{
		"name": "neo", "role": "Renamed Role", "description": "The developer",
		"category": "Engineering", "long_description": neoLongDescription,
	})
	fixture.put("catalog.nims.neo", 99, string(neo))

	cached, _ := service.Bundle("neo")
	if cached.Identity.Source.Revision != first.Identity.Source.Revision {
		t.Fatal("bundle was not served from cache")
	}

	watcher.fire("catalog.nims.neo")

	fresh, err := service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Identity.Role != "Renamed Role" || fresh.Identity.Source.Revision != 99 {
		t.Errorf("bundle not regenerated: %+v", fresh.Identity)
	}

	// A shared-catalog change clears every bundle.
	fixture.put("catalog.tools.bash", 44, `{"name":"Bash","description":"Execute shell commands","assigned_nims":["neo"]}`)
	watcher.fire("catalog.tools.bash")
	fresh2, _ := service.Bundle("neo")
	if fresh2.Policy.Tools[0].Source.Revision != 44 {
		t.Errorf("tool revision = %d, want 44", fresh2.Policy.Tools[0].Source.Revision)
	}
}

func TestWatchInvalidatesRoleList(t *testing.T) {
	fixture := fixtureSoil()
	service := newTestService(fixture)
	watcher := &fakeWatcher{}
	stop, err := service.StartWatch(watcher)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	list, err := service.ListRoles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d roles, want 2", len(list))
	}

	fixture.put("catalog.nims.notary", 80, `{"name":"notary","role":"Records","description":"Keeps records","category":"Operations"}`)
	watcher.fire("catalog.nims.notary")

	list, err = service.ListRoles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d roles after create, want 3", len(list))
	}
}

// fakeRegistry serves the nimregistry API shape for fallback tests.
func fakeRegistry(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/nims", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"neo","role":"Software Development","description":"The developer","category":"Engineering","long_description":` + mustJSON(neoLongDescription) + `}]`))
	})
	mux.HandleFunc("GET /api/nims/neo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"neo","role":"Software Development","description":"The developer","category":"Engineering","long_description":` + mustJSON(neoLongDescription) + `}`))
	})
	mux.HandleFunc("GET /api/nims/neo/prompt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte("# Neo prompt"))
	})
	mux.HandleFunc("GET /api/nims/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"nim not found"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestFallbackToRegistryWhenSoilDisabled(t *testing.T) {
	registry := fakeRegistry(t)
	service := NewService(soil.Disabled{Reason: "no nats"}, NewRegistryClient(registry.URL))

	if !service.Degraded() {
		t.Fatal("service does not report degraded")
	}

	list, err := service.ListRoles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "neo" {
		t.Fatalf("list = %+v", list)
	}
	if list[0].Source.Revision != 0 || list[0].Source.Origin != OriginRegistry {
		t.Errorf("fallback source = %+v", list[0].Source)
	}

	bundle, err := service.Bundle("neo")
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.Degraded {
		t.Error("fallback bundle not marked degraded")
	}
	if bundle.Identity.Source.Revision != 0 || bundle.Identity.Source.Origin != OriginRegistry {
		t.Errorf("identity source = %+v", bundle.Identity.Source)
	}
	if bundle.Identity.Prompt != "# Neo prompt" {
		t.Errorf("prompt = %q", bundle.Identity.Prompt)
	}
	// Assignments live only in Soil: honest empty slots.
	if len(bundle.Policy.Tools) != 0 || len(bundle.Skills) != 0 {
		t.Errorf("degraded bundle invented assignments: %+v", bundle)
	}
	// Acceptance still derives from long_description.
	if bundle.Acceptance.State != AcceptanceParsed || len(bundle.Acceptance.Criteria) != 2 {
		t.Errorf("acceptance = %+v", bundle.Acceptance)
	}

	if _, err := service.Bundle("nobody"); err != ErrRoleNotFound {
		t.Fatalf("err = %v, want ErrRoleNotFound", err)
	}
}
