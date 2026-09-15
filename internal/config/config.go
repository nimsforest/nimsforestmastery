// Package config loads the portal configuration from flags and the
// environment. Environment variables override flags, the same order the
// other NimsForest services use.
package config

import (
	"flag"
	"fmt"
	tool "github.com/nimsforest/nimsforesttool"
	"os"
	"strings"
)

// Defaults. The port is the portal's own, free across the land seeds (8110 is taken by callyouragentai); nimregistry serves on 8101 on
// the same land.
const (
	DefaultPort           = "8111"
	DefaultNATSURL        = "nats://127.0.0.1:4222"
	DefaultNimRegistryURL = "http://127.0.0.1:8101"
	DefaultContentDir     = "./content"
)

// Config holds everything the service needs at startup.
type Config struct {
	// OrgSlug is the -org flag value only. The environment variable
	// ORG_SLUG is the authority and is read by tool.RequireOrg, which
	// receives this flag value as the expected organization: a flag
	// that disagrees with ORG_SLUG fails fast instead of being
	// silently overridden.
	OrgSlug string

	// Listen is the HTTP listen address, ":" + PORT.
	Listen string

	// NATSURL is the forest bus. The service degrades cleanly when it
	// is unreachable.
	NATSURL string

	// IamNimURL is the identity service. It has no default on purpose:
	// a wrong default would silently point a test land at production
	// identity. The web layer fails closed when it is empty.
	IamNimURL string

	// NimRegistryURL is the HTTP fallback read path when Soil is not
	// available.
	NimRegistryURL string

	// BaseURL is the portal's own external URL, used as the iamnim
	// login return target. The human role pages fail closed when it is
	// empty.
	BaseURL string

	// ContentDir holds the on-disk shared content, the doctrine
	// curriculum under <ContentDir>/doctrine. The content is a COPY in
	// the image, never a go:embed.
	ContentDir string
}

// Load parses flags from args and applies environment overrides.
// Recognized environment variables: PORT, NATS_URL, IAMNIM_URL,
// NIMREGISTRY_URL, BASE_URL, CONTENT_DIR. ORG_SLUG is read by
// tool.RequireOrg, never applied over the -org flag.
func Load(args []string) (*Config, error) {
	fs := flag.NewFlagSet("nimsforestmastery", flag.ContinueOnError)
	orgSlug := fs.String("org", "", "organization slug (must agree with ORG_SLUG)")
	port := fs.String("port", DefaultPort, "HTTP port")
	natsURL := fs.String("nats", DefaultNATSURL, "NATS server URL")
	iamnimURL := fs.String("iamnim", "", "iamnim base URL (no default on purpose)")
	nimregistryURL := fs.String("nimregistry", DefaultNimRegistryURL, "nimregistry HTTP API base URL")
	baseURL := fs.String("base-url", "", "the portal's own external URL, the iamnim login return target")
	contentDir := fs.String("content", DefaultContentDir, "directory with the shared doctrine content")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	applyEnv := func(name string, target *string) {
		if v := os.Getenv(name); v != "" {
			*target = v
		}
	}
	// ORG_SLUG is deliberately NOT applied here: tool.RequireOrg
	// reads it and checks it against the -org flag value, so a
	// disagreeing flag fails fast instead of being overridden.
	applyEnv("PORT", port)
	applyEnv("NATS_URL", natsURL)
	applyEnv("IAMNIM_URL", iamnimURL)
	applyEnv("NIMREGISTRY_URL", nimregistryURL)
	applyEnv("BASE_URL", baseURL)
	applyEnv("CONTENT_DIR", contentDir)

	if *port == "" {
		return nil, fmt.Errorf("config: PORT must not be empty")
	}

	return &Config{
		OrgSlug: *orgSlug,
		// Placement belongs to the role. LISTEN carries a full address, so
		// unlike PORT it can also name an interface; the port flag and its
		// PORT override stay as the fallback.
		Listen:         tool.ListenAddr("", ":"+*port),
		NATSURL:        *natsURL,
		IamNimURL:      *iamnimURL,
		NimRegistryURL: *nimregistryURL,
		BaseURL:        strings.TrimRight(*baseURL, "/"),
		ContentDir:     *contentDir,
	}, nil
}
