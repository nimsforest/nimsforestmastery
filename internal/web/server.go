// Package web serves the human surface of the mastery portal with the
// shared nwc frame: the public role directory and doctrine curriculum,
// and the SSO-protected role pages Meet, Learn and the changelog.
// Every role page is generated at request time from the live catalog;
// nothing here is authored per nim.
package web

import (
	"errors"
	"log"
	"net/http"
	"strings"

	nwc "github.com/nimsforest/nimsforestwebcomponents"

	"github.com/nimsforest/nimsforestmastery/internal/doctrine"
	"github.com/nimsforest/nimsforestmastery/internal/roles"
)

// Server renders the human pages. Public() serves the routes that
// need no credential; RolePages() serves the role paths and must be
// wrapped in the Auth middleware by the caller.
type Server struct {
	service   *roles.Service
	doctrine  *doctrine.Store
	renderer  *nwc.Renderer
	org       string
	version   string
	public    *http.ServeMux
	rolePages *http.ServeMux
}

// NewServer builds the human surface over the role service and the
// doctrine store.
func NewServer(service *roles.Service, store *doctrine.Store, org, version string) *Server {
	s := &Server{
		service:   service,
		doctrine:  store,
		org:       org,
		version:   version,
		public:    http.NewServeMux(),
		rolePages: http.NewServeMux(),
	}

	s.renderer = nwc.NewRendererWithShared(templateFS, "templates",
		[]string{
			"index.html",
			"doctrine.html",
			"doctrine_unit.html",
			"role_meet.html",
			"role_learn.html",
			"role_changelog.html",
			"notice.html",
		},
		[]string{"role_shared.html"},
		nil,
		nwc.AppConfig{
			Name: "Mastery",
			NavItems: []nwc.NavItem{
				{Label: "Roles", Href: "/"},
				{Label: "Doctrine", Href: "/doctrine"},
			},
			Footer: "NimsForest Mastery",
		})

	s.public.HandleFunc("GET /{$}", s.handleIndex)
	s.public.HandleFunc("GET /doctrine", s.handleDoctrineIndex)
	s.public.HandleFunc("GET /doctrine/{$}", s.handleDoctrineIndex)
	s.public.HandleFunc("GET /doctrine/{unit}", s.handleDoctrineUnit)

	s.rolePages.HandleFunc("GET /roles/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	s.rolePages.HandleFunc("GET /roles/{name}", s.handleRoleMeet)
	s.rolePages.HandleFunc("GET /roles/{name}/learn", s.handleRoleLearn)
	s.rolePages.HandleFunc("GET /roles/{name}/changelog", s.handleRoleChangelog)

	return s
}

// DispatchRoles splits /roles/ between the two surfaces: exactly
// /roles/<name>.md is the agent markdown page (Bearer auth, JSON
// errors); every other role path is a human page behind the SSO
// redirect flow. Both handlers arrive already wrapped in their auth.
func DispatchRoles(agent, human http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/roles/")
		if strings.HasSuffix(rest, ".md") && !strings.Contains(rest, "/") {
			agent.ServeHTTP(w, r)
			return
		}
		human.ServeHTTP(w, r)
	})
}

// Public returns the handler for the routes that need no credential:
// the role directory and the doctrine curriculum. These pages carry
// role summaries and shared doctrine only, never role content.
func (s *Server) Public() http.Handler { return s.public }

// RolePages returns the handler for the role paths. The caller MUST
// wrap it in the Auth middleware; it is never mounted bare.
func (s *Server) RolePages() http.Handler { return s.rolePages }

// render writes one page with an explicit status code.
func (s *Server) render(w http.ResponseWriter, status int, page, title string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	s.renderer.Render(w, page, title, data)
}

// noticeData is the payload of the shared notice page.
type noticeData struct {
	Heading string
	Message string
}

// renderRoleError maps a domain error to an honest page: 404 for an
// unknown role, 503 with the reason when the catalog is unreachable,
// never a stale render and never a silent empty page.
func (s *Server) renderRoleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, roles.ErrRoleNotFound):
		s.render(w, http.StatusNotFound, "notice.html", "Role not found", noticeData{
			Heading: "Role not found",
			Message: "No nim with this name exists in the catalog. The role directory lists every live role.",
		})
	case errors.Is(err, roles.ErrUnavailable):
		s.render(w, http.StatusServiceUnavailable, "notice.html", "Catalog unavailable", noticeData{
			Heading: "Role catalog unavailable",
			Message: "Neither Soil nor the nimregistry fallback answered: " + err.Error() +
				". The portal never serves a stale render; retry shortly.",
		})
	default:
		log.Printf("web: %v", err)
		s.render(w, http.StatusInternalServerError, "notice.html", "Error", noticeData{
			Heading: "Something went wrong",
			Message: "The page could not be generated. The error is logged.",
		})
	}
}

// indexData is the payload of the public role directory.
type indexData struct {
	Org      string
	Degraded bool
	Roles    []roles.RoleSummary
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	list, err := s.service.ListRoles()
	if err != nil {
		s.renderRoleError(w, err)
		return
	}
	s.render(w, http.StatusOK, "index.html", "Roles", indexData{
		Org:      s.org,
		Degraded: s.service.Degraded(),
		Roles:    list,
	})
}

// doctrineIndexData is the payload of the doctrine index.
type doctrineIndexData struct {
	Units []doctrine.Unit
}

func (s *Server) handleDoctrineIndex(w http.ResponseWriter, r *http.Request) {
	units, err := s.doctrine.Units()
	if err != nil {
		log.Printf("web: doctrine: %v", err)
		s.render(w, http.StatusInternalServerError, "notice.html", "Error", noticeData{
			Heading: "Doctrine unavailable",
			Message: "The doctrine content could not be read. The error is logged.",
		})
		return
	}
	s.render(w, http.StatusOK, "doctrine.html", "Doctrine", doctrineIndexData{Units: units})
}

// doctrineUnitData is the payload of one doctrine unit page.
type doctrineUnitData struct {
	Unit doctrine.Unit
}

func (s *Server) handleDoctrineUnit(w http.ResponseWriter, r *http.Request) {
	unit, err := s.doctrine.Unit(r.PathValue("unit"))
	if errors.Is(err, doctrine.ErrNotFound) {
		s.render(w, http.StatusNotFound, "notice.html", "Unit not found", noticeData{
			Heading: "Doctrine unit not found",
			Message: "No doctrine unit with this name exists. The doctrine index lists every unit.",
		})
		return
	}
	if err != nil {
		log.Printf("web: doctrine: %v", err)
		s.render(w, http.StatusInternalServerError, "notice.html", "Error", noticeData{
			Heading: "Doctrine unavailable",
			Message: "The doctrine content could not be read. The error is logged.",
		})
		return
	}
	s.render(w, http.StatusOK, "doctrine_unit.html", unit.Title, doctrineUnitData{Unit: unit})
}

// rolePageData is the payload shared by the three role pages.
type rolePageData struct {
	Bundle    *roles.RoleBundle
	ActiveTab string

	// Learn page extras.
	DoctrineUnits []doctrine.Unit
	DataNote      string

	// Changelog page extras.
	Sources []sourceRow
}

// sourceRow is one line of the changelog revision table.
type sourceRow struct {
	Slot     string
	Key      string
	Revision uint64
	Origin   string
}

func (s *Server) handleRoleMeet(w http.ResponseWriter, r *http.Request) {
	bundle, err := s.service.Bundle(r.PathValue("name"))
	if err != nil {
		s.renderRoleError(w, err)
		return
	}
	s.render(w, http.StatusOK, "role_meet.html", bundle.Name, rolePageData{
		Bundle:    bundle,
		ActiveTab: "meet",
	})
}

func (s *Server) handleRoleLearn(w http.ResponseWriter, r *http.Request) {
	bundle, err := s.service.Bundle(r.PathValue("name"))
	if err != nil {
		s.renderRoleError(w, err)
		return
	}
	units, err := s.doctrine.Units()
	if err != nil {
		log.Printf("web: doctrine: %v", err)
		units = nil
	}
	s.render(w, http.StatusOK, "role_learn.html", bundle.Name+" learn", rolePageData{
		Bundle:        bundle,
		ActiveTab:     "learn",
		DoctrineUnits: units,
		DataNote:      dataContextNote(bundle.Identity.Category),
	})
}

func (s *Server) handleRoleChangelog(w http.ResponseWriter, r *http.Request) {
	bundle, err := s.service.Bundle(r.PathValue("name"))
	if err != nil {
		s.renderRoleError(w, err)
		return
	}
	s.render(w, http.StatusOK, "role_changelog.html", bundle.Name+" changelog", rolePageData{
		Bundle:    bundle,
		ActiveTab: "changelog",
		Sources:   sourceRows(bundle),
	})
}

// dataContextNote is the Learn data unit, keyed on the role's live
// catalog category, never on compiled nim names: a renamed or added
// growth role keeps the right note without a portal change.
func dataContextNote(category string) string {
	if strings.EqualFold(category, "growth") {
		return "This role works with the organization's growth context: growth reports plus the catalog.organize.* and catalog.productize.* knowledge in Soil."
	}
	return "Organization knowledge for this role comes from the catalog.organize.* and catalog.productize.* entries in Soil."
}

// sourceRows flattens every source stamp of a bundle into the
// changelog revision table.
func sourceRows(bundle *roles.RoleBundle) []sourceRow {
	row := func(slot string, ref roles.SourceRef) sourceRow {
		return sourceRow{Slot: slot, Key: ref.Key, Revision: ref.Revision, Origin: originLabel(ref)}
	}
	rows := []sourceRow{
		row("Identity", bundle.Identity.Source),
		row("Behavior prompt", bundle.Identity.PromptSource),
	}
	for _, ceremony := range bundle.Identity.CeremonyPrompts {
		rows = append(rows, row("Ceremony "+ceremony.Name, ceremony.Source))
	}
	rows = append(rows,
		row("Backing humans", bundle.Identity.BackingSource),
		row("Policy config", bundle.Policy.ConfigSource),
	)
	for _, tool := range bundle.Policy.Tools {
		rows = append(rows, row("Tool "+tool.Name, tool.Source))
	}
	for _, skill := range bundle.Skills {
		rows = append(rows, row("Skill "+skill.Name, skill.Source))
		rows = append(rows, row("Skill content "+skill.Name, skill.ContentSource))
	}
	rows = append(rows, row("Acceptance", bundle.Acceptance.Source))
	return rows
}

func originLabel(ref roles.SourceRef) string {
	switch {
	case ref.Key == "":
		return "no source"
	case ref.Origin == roles.OriginSoil:
		return "soil"
	case ref.Origin == roles.OriginRegistry:
		return "nimregistry fallback"
	default:
		return "not present in soil"
	}
}
