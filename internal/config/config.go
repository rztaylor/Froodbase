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
}

// Dreamer is the startup configuration of froodbase-dreamer.
type Dreamer struct {
	// DatabaseURL is the PostgreSQL connection URL.
	DatabaseURL string
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
	return API{Addr: addr, DatabaseURL: dbURL}, nil
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
