package config

import (
	"strings"
	"testing"
	"time"
)

func mapLookup(m map[string]string) Lookup {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

const goodURL = "postgres://froodbase:s3cret@postgres:5432/froodbase?sslmode=disable"

const usersFile = "/etc/froodbase/users.json"

func TestLoadAPI(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    API
		wantErr string
	}{
		{
			name: "defaults the address",
			env:  map[string]string{EnvDatabaseURL: goodURL, EnvUsersFile: usersFile},
			want: API{Addr: DefaultAPIAddr, DatabaseURL: goodURL, UsersFile: usersFile},
		},
		{
			name: "empty address is the default",
			env:  map[string]string{EnvDatabaseURL: goodURL, EnvAPIAddr: "", EnvUsersFile: usersFile},
			want: API{Addr: DefaultAPIAddr, DatabaseURL: goodURL, UsersFile: usersFile},
		},
		{
			name: "explicit address",
			env:  map[string]string{EnvDatabaseURL: goodURL, EnvAPIAddr: "127.0.0.1:9090", EnvUsersFile: usersFile},
			want: API{Addr: "127.0.0.1:9090", DatabaseURL: goodURL, UsersFile: usersFile},
		},
		{
			name: "postgresql scheme",
			env:  map[string]string{EnvDatabaseURL: "postgresql://u:p@h/db", EnvUsersFile: usersFile},
			want: API{Addr: DefaultAPIAddr, DatabaseURL: "postgresql://u:p@h/db", UsersFile: usersFile},
		},
		{
			name:    "users file missing",
			env:     map[string]string{EnvDatabaseURL: goodURL},
			wantErr: "FROODBASE_USERS_FILE is not set",
		},
		{
			name:    "database URL missing",
			env:     map[string]string{},
			wantErr: "FROODBASE_DATABASE_URL is not set",
		},
		{
			name:    "database URL empty",
			env:     map[string]string{EnvDatabaseURL: ""},
			wantErr: "FROODBASE_DATABASE_URL is not set",
		},
		{
			name:    "database URL wrong scheme",
			env:     map[string]string{EnvDatabaseURL: "mysql://u:p@h/db"},
			wantErr: "must start with postgres://",
		},
		{
			name:    "database URL without host",
			env:     map[string]string{EnvDatabaseURL: "postgres:///db"},
			wantErr: "has no host",
		},
		{
			name:    "database URL unparsable",
			env:     map[string]string{EnvDatabaseURL: "postgres://u:p@h:port/db"},
			wantErr: "is not a valid URL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadAPI(mapLookup(tt.env))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("LoadAPI() = %+v, want error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadAPI() error = %q, want it to contain %q", err, tt.wantErr)
				}
				if strings.Contains(err.Error(), "s3cret") {
					t.Fatalf("LoadAPI() error leaks the password: %q", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadAPI() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("LoadAPI() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadDreamer(t *testing.T) {
	got, err := LoadDreamer(mapLookup(map[string]string{EnvDatabaseURL: goodURL}))
	if err != nil {
		t.Fatalf("LoadDreamer() error = %v", err)
	}
	if got.DatabaseURL != goodURL {
		t.Fatalf("LoadDreamer().DatabaseURL = %q, want %q", got.DatabaseURL, goodURL)
	}

	_, err = LoadDreamer(mapLookup(map[string]string{}))
	if err == nil || !strings.Contains(err.Error(), "FROODBASE_DATABASE_URL is not set") {
		t.Fatalf("LoadDreamer() with no URL: error = %v, want 'is not set'", err)
	}
}

func TestLoadDreamerReviewSweepInterval(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		set     bool
		want    time.Duration
		wantErr string
	}{
		{"unset uses the default", "", false, DefaultReviewSweepInterval, ""},
		{"empty uses the default", "", true, DefaultReviewSweepInterval, ""},
		{"fifteen minutes", "15m", true, 15 * time.Minute, ""},
		{"the minimum", "1m", true, time.Minute, ""},
		{"the maximum", "24h", true, 24 * time.Hour, ""},
		{"surrounding spaces", " 2h ", true, 2 * time.Hour, ""},
		{"below the minimum", "30s", true, 0, "FROODBASE_REVIEW_SWEEP_INTERVAL must be between 1m0s and 24h0m0s"},
		{"above the maximum", "25h", true, 0, "FROODBASE_REVIEW_SWEEP_INTERVAL must be between 1m0s and 24h0m0s"},
		{"negative", "-1h", true, 0, "must be between"},
		{"a bare number", "60", true, 0, "FROODBASE_REVIEW_SWEEP_INTERVAL must be a duration such as 1h or 30m"},
		{"nonsense", "hourly", true, 0, "must be a duration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{EnvDatabaseURL: goodURL}
			if tt.set {
				env[EnvReviewSweepInterval] = tt.value
			}
			got, err := LoadDreamer(mapLookup(env))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadDreamer() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadDreamer() error = %v", err)
			}
			if got.ReviewSweepInterval != tt.want {
				t.Errorf("ReviewSweepInterval = %v, want %v", got.ReviewSweepInterval, tt.want)
			}
		})
	}
	if DefaultReviewSweepInterval != time.Hour {
		t.Errorf("DefaultReviewSweepInterval = %v, want 1h (documented in docs/dev/ops/development.md)", DefaultReviewSweepInterval)
	}
}

func TestLoadMCP(t *testing.T) {
	full := map[string]string{EnvAPIURL: "http://127.0.0.1:8080/mcp", EnvUsername: "alice", EnvPassword: "s3cret"}
	got, err := LoadMCP(mapLookup(full))
	if err != nil || got.APIURL != full[EnvAPIURL] || got.Username != "alice" || got.Password != "s3cret" {
		t.Fatalf("LoadMCP() = %+v, %v", got, err)
	}
	for _, missing := range []string{EnvAPIURL, EnvUsername, EnvPassword} {
		env := map[string]string{}
		for k, v := range full {
			if k != missing {
				env[k] = v
			}
		}
		_, err := LoadMCP(mapLookup(env))
		if err == nil || !strings.Contains(err.Error(), missing+" is not set") {
			t.Errorf("without %s: err = %v", missing, err)
		}
		if err != nil && strings.Contains(err.Error(), "s3cret") {
			t.Errorf("error echoes the password: %v", err)
		}
	}
	env := map[string]string{EnvAPIURL: "ftp://x/mcp", EnvUsername: "a", EnvPassword: "b"}
	if _, err := LoadMCP(mapLookup(env)); err == nil || !strings.Contains(err.Error(), "http://") {
		t.Errorf("ftp URL: err = %v", err)
	}
}
