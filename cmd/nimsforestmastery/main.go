// Command nimsforestmastery runs the role mastery portal for one
// organization. It generates role bundles live from the forest's own
// Soil, with the nimregistry HTTP API as the fallback read path.
// Design: nimsforest2 docs/architecture/ROLE_MASTERY.md.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	nwc "github.com/nimsforest/nimsforestwebcomponents"

	"github.com/nimsforest/nimsforestmastery/internal/api"
	"github.com/nimsforest/nimsforestmastery/internal/config"
	"github.com/nimsforest/nimsforestmastery/internal/doctrine"
	"github.com/nimsforest/nimsforestmastery/internal/roles"
	"github.com/nimsforest/nimsforestmastery/internal/soil"
	"github.com/nimsforest/nimsforestmastery/internal/tool"
	"github.com/nimsforest/nimsforestmastery/internal/web"
)

// version is set at build time via ldflags.
var version = "dev"

func main() {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		log.Fatalf("%s: %v", tool.Name, err)
	}

	// Single tenancy: ORG_SLUG is the authority and must be set. The
	// -org flag value is passed as the expected organization, so a
	// disagreeing flag fails fast.
	org, err := tool.RequireOrg(cfg.OrgSlug)
	if err != nil {
		log.Fatalf("%s: %v", tool.Name, err)
	}
	cfg.OrgSlug = org
	log.Printf("%s %s starting for organization %s", tool.Name, version, org)

	if cfg.IamNimURL == "" {
		log.Printf("warning: IAMNIM_URL not set, authenticated routes answer 503 (fail closed)")
	}
	if cfg.BaseURL == "" {
		log.Printf("warning: BASE_URL not set, the human role pages answer 503 (fail closed)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Soil is read live. When NATS or the bucket is absent the portal
	// runs in a disabled Soil state and says so, instead of serving
	// stale content or refusing to start.
	var reader soil.Reader
	var joiner *tool.Joiner
	var stopWatch func()

	nc, err := soil.Connect(cfg.NATSURL, tool.Name)
	if err != nil {
		log.Printf("warning: NATS unreachable at %s, running with Soil disabled: %v", cfg.NATSURL, err)
		reader = soil.Disabled{Reason: err.Error()}
	} else {
		defer nc.Close()
		kv, err := soil.Open(nc)
		if err != nil {
			log.Printf("warning: Soil bucket unavailable, running with Soil disabled: %v", err)
			reader = soil.Disabled{Reason: err.Error()}
		} else {
			reader = kv
		}

		// Joining contract: announce, heartbeat, deregister.
		joiner, err = tool.Join(nc, org, version)
		if err != nil {
			log.Printf("warning: mycelium register failed: %v", err)
		}
	}

	registry := roles.NewRegistryClient(cfg.NimRegistryURL)
	service := roles.NewService(reader, registry)
	if kv, ok := reader.(*soil.KV); ok {
		stopWatch, err = service.StartWatch(kv)
		if err != nil {
			// Without watches a cached render could go stale silently,
			// which the anti-drift rule forbids: turn the cache off and
			// serve read-through on every request instead.
			service.DisableCache()
			log.Printf("warning: soil watch failed, render cache disabled, serving read-through: %v", err)
		}
	}

	// Two surfaces, one source. The agent surface answers JSON and
	// markdown behind PAT Bearer auth; the human surface answers nwc
	// pages, with the role pages behind the iamnim SSO redirect flow.
	// Health, static assets, the role directory and the doctrine
	// curriculum are public; public pages carry no role content.
	agentAuth := api.NewAuth(cfg.IamNimURL, org)
	agentRoutes := agentAuth.Wrap(api.NewServer(service, org, version))

	webServer := web.NewServer(service, doctrine.NewStore(cfg.ContentDir), org, version)
	webAuth := web.NewAuth(cfg.IamNimURL, org, cfg.BaseURL)
	roleWebRoutes := webAuth.Wrap(webServer.RolePages())

	mux := http.NewServeMux()
	// Both paths are part of the contract: a Land probe reads one, the
	// observability plane the other.
	health := tool.HealthHandler(reader, registry, cfg.IamNimURL)
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /api/v1/health", health)
	mux.Handle("GET /static/", http.StripPrefix("/static/", nwc.StaticHandler()))
	mux.Handle("/api/", agentRoutes)
	mux.Handle("GET /llms.txt", agentRoutes)
	mux.Handle("/roles/", web.DispatchRoles(agentRoutes, roleWebRoutes))
	mux.Handle("/", webServer.Public())

	runServer(ctx, cfg.Listen, mux)
	shutdown(joiner, stopWatch)
}

func runServer(ctx context.Context, listen string, handler http.Handler) {
	server := &http.Server{Addr: listen, Handler: handler}
	go func() {
		log.Printf("%s listening on %s", tool.Name, listen)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("%s: serve: %v", tool.Name, err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("warning: http shutdown: %v", err)
	}
}

func shutdown(joiner *tool.Joiner, stopWatch func()) {
	if stopWatch != nil {
		stopWatch()
	}
	joiner.Leave()
}
