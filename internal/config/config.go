package config

import (
	"fmt"
	"net/url"
)

// Environment variable names read by the stack. Every one must appear in
// .env.example and docs/dev/ops/development.md.
const (
	EnvAPIAddr     = "FROODBASE_API_ADDR"
	EnvDatabaseURL = "FROODBASE_DATABASE_URL"
	EnvUsersFile   = "FROODBASE_USERS_FILE"
	EnvAPIURL      = "FROODBASE_API_URL"
	EnvUsername    = "FROODBASE_USERNAME"
	EnvPassword    = "FROODBASE_PASSWORD"
)

// DefaultAPIAddr is the listen address froodbase-api uses when
// FROODBASE_API_ADDR is unset.
const DefaultAPIAddr = ":8080"

// Lookup reads one environment variable. os.LookupEnv satisfies it; tests pass
// a map-backed function.
type Lookup func(key string) (value string, ok bool)

// API is the startup configuration of froodbase-api.
type API struct {
	// Addr is the TCP address the HTTP server listens on, host:port or :port.
	Addr string
	// DatabaseURL is the PostgreSQL connection URL.
	DatabaseURL string
	// UsersFile is the path of the static users file (local MVP auth).
	UsersFile string
}

// Dreamer is the startup configuration of froodbase-dreamer.
type Dreamer struct {
	// DatabaseURL is the PostgreSQL connection URL.
	DatabaseURL string
}

// MCP is the startup configuration of froodbase-mcp.
type MCP struct {
	// APIURL is the froodbase-api MCP endpoint, for example
	// http://127.0.0.1:8080/mcp.
	APIURL string
	// Username and Password authenticate the person to froodbase-api.
	Username string
	Password string
}

// LoadMCP reads and validates the froodbase-mcp configuration. Errors name
// the variable, never its value.
func LoadMCP(lookup Lookup) (MCP, error) {
	apiURL, ok := lookup(EnvAPIURL)
	if !ok || apiURL == "" {
		return MCP{}, fmt.Errorf("config: %s is not set", EnvAPIURL)
	}
	u, err := url.Parse(apiURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return MCP{}, fmt.Errorf("config: %s must be an http:// or https:// URL", EnvAPIURL)
	}
	user, ok := lookup(EnvUsername)
	if !ok || user == "" {
		return MCP{}, fmt.Errorf("config: %s is not set", EnvUsername)
	}
	pass, ok := lookup(EnvPassword)
	if !ok || pass == "" {
		return MCP{}, fmt.Errorf("config: %s is not set", EnvPassword)
	}
	return MCP{APIURL: apiURL, Username: user, Password: pass}, nil
}

// LoadAPI reads and validates the froodbase-api configuration.
func LoadAPI(lookup Lookup) (API, error) {
	addr, ok := lookup(EnvAPIAddr)
	if !ok || addr == "" {
		addr = DefaultAPIAddr
	}
	dbURL, err := databaseURL(lookup)
	if err != nil {
		return API{}, err
	}
	users, ok := lookup(EnvUsersFile)
	if !ok || users == "" {
		return API{}, fmt.Errorf("config: %s is not set", EnvUsersFile)
	}
	return API{Addr: addr, DatabaseURL: dbURL, UsersFile: users}, nil
}

// LoadMigrate reads the configuration froodbase-api migrate needs: the
// database URL only.
func LoadMigrate(lookup Lookup) (API, error) {
	dbURL, err := databaseURL(lookup)
	if err != nil {
		return API{}, err
	}
	return API{DatabaseURL: dbURL}, nil
}

// LoadDreamer reads and validates the froodbase-dreamer configuration.
func LoadDreamer(lookup Lookup) (Dreamer, error) {
	dbURL, err := databaseURL(lookup)
	if err != nil {
		return Dreamer{}, err
	}
	return Dreamer{DatabaseURL: dbURL}, nil
}

// databaseURL requires FROODBASE_DATABASE_URL to be a postgres:// or
// postgresql:// URL. The value is never included in an error because it
// carries the password.
func databaseURL(lookup Lookup) (string, error) {
	raw, ok := lookup(EnvDatabaseURL)
	if !ok || raw == "" {
		return "", fmt.Errorf("config: %s is not set", EnvDatabaseURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("config: %s is not a valid URL", EnvDatabaseURL)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("config: %s must start with postgres:// or postgresql://", EnvDatabaseURL)
	}
	if u.Host == "" {
		return "", fmt.Errorf("config: %s has no host", EnvDatabaseURL)
	}
	return raw, nil
}
