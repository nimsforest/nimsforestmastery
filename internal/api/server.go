package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/nimsforest/nimsforestmastery/internal/roles"
)

// Server is the agent surface. Routes:
//
//	GET /api/v1/roles                 role directory with revisions
//	GET /api/v1/roles/{name}/bundle   the six-slot bundle, stamped
//	GET /roles/{name}.md              the whole path as one markdown
//	                                  document; the primary agent
//	                                  product a Claude session reads
//	GET /llms.txt                     the agent index
//
// The caller wraps the whole server in the Auth middleware; the
// health endpoint lives outside it.
type Server struct {
	service *roles.Service
	org     string
	version string
	mux     *http.ServeMux
}

// NewServer builds the agent surface over the role service.
func NewServer(service *roles.Service, org, version string) *Server {
	s := &Server{service: service, org: org, version: version, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /api/v1/roles", s.handleListRoles)
	s.mux.HandleFunc("GET /api/v1/roles/{name}/bundle", s.handleBundle)
	s.mux.HandleFunc("GET /roles/{page}", s.handleRoleMarkdown)
	s.mux.HandleFunc("GET /llms.txt", s.handleLLMsTxt)
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// listResponse is the /api/v1/roles wire shape.
type listResponse struct {
	Organization string              `json:"organization"`
	Degraded     bool                `json:"degraded"`
	Roles        []roles.RoleSummary `json:"roles"`
}

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	list, err := s.service.ListRoles()
	if err != nil {
		s.writeRoleError(w, err)
		return
	}
	if list == nil {
		list = []roles.RoleSummary{}
	}
	writeJSON(w, http.StatusOK, listResponse{
		Organization: s.org,
		Degraded:     s.service.Degraded(),
		Roles:        list,
	})
}

func (s *Server) handleBundle(w http.ResponseWriter, r *http.Request) {
	bundle, err := s.service.Bundle(r.PathValue("name"))
	if err != nil {
		s.writeRoleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) handleRoleMarkdown(w http.ResponseWriter, r *http.Request) {
	page := r.PathValue("page")
	name, ok := strings.CutSuffix(page, ".md")
	if !ok || name == "" {
		writeJSONError(w, http.StatusNotFound, "not found",
			"the agent role page is /roles/<name>.md")
		return
	}
	bundle, err := s.service.Bundle(name)
	if err != nil {
		s.writeRoleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = io.WriteString(w, RenderRoleMarkdown(bundle, s.org))
}

func (s *Server) handleLLMsTxt(w http.ResponseWriter, r *http.Request) {
	list, err := s.service.ListRoles()
	if err != nil {
		s.writeRoleError(w, err)
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s role mastery\n\n", s.org)
	fmt.Fprintf(&sb, "> Live role bundles for the %s forest. One role per nim,\n", s.org)
	sb.WriteString("> generated from the catalog at request time. Point a Claude\n")
	sb.WriteString("> session at /roles/<name>.md to take on a role.\n\n")
	sb.WriteString("## Roles\n\n")
	for _, role := range list {
		fmt.Fprintf(&sb, "- [%s](/roles/%s.md): %s. %s\n", role.Name, role.Name, role.Role, role.Description)
	}
	sb.WriteString("\n## API\n\n")
	sb.WriteString("- [/api/v1/roles](/api/v1/roles): role directory with source revisions\n")
	sb.WriteString("- /api/v1/roles/<name>/bundle: the six-slot role bundle, revision stamped\n")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, sb.String())
}

// writeRoleError maps domain errors to honest HTTP answers. A catalog
// that is fully unreachable answers 503 with the reason, never a
// stale render and never a silent empty list.
func (s *Server) writeRoleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, roles.ErrRoleNotFound):
		writeJSONError(w, http.StatusNotFound, "role not found", "")
	case errors.Is(err, roles.ErrUnavailable):
		writeJSONError(w, http.StatusServiceUnavailable, "role catalog unavailable", err.Error())
	default:
		log.Printf("api: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error", "")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONError answers one error as JSON. The reason is optional
// detail, for example why the catalog is unavailable.
func writeJSONError(w http.ResponseWriter, status int, message, reason string) {
	body := map[string]string{"error": message}
	if reason != "" {
		body["reason"] = reason
	}
	writeJSON(w, status, body)
}
