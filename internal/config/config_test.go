package config

import (
	"strings"
	"testing"
)

func mapLookup(m map[string]string) Lookup {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

const goodURL = "postgres://froodbase:s3cret@postgres:5432/froodbase?sslmode=disable"

func TestLoadAPI(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    API
		wantErr string
	}{
		{
			name: "defaults the address",
			env:  map[string]string{EnvDatabaseURL: goodURL},
			want: API{Addr: DefaultAPIAddr, DatabaseURL: goodURL},
		},
		{
			name: "empty address is the default",
			env:  map[string]string{EnvDatabaseURL: goodURL, EnvAPIAddr: ""},
			want: API{Addr: DefaultAPIAddr, DatabaseURL: goodURL},
		},
		{
			name: "explicit address",
			env:  map[string]string{EnvDatabaseURL: goodURL, EnvAPIAddr: "127.0.0.1:9090"},
			want: API{Addr: "127.0.0.1:9090", DatabaseURL: goodURL},
		},
		{
			name: "postgresql scheme",
			env:  map[string]string{EnvDatabaseURL: "postgresql://u:p@h/db"},
			want: API{Addr: DefaultAPIAddr, DatabaseURL: "postgresql://u:p@h/db"},
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
