package store

import (
	"errors"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"testing"
)

func TestEmbeddedMigrations(t *testing.T) {
	entries, err := fs.ReadDir(Migrations(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded migrations")
	}
	last := int64(0)
	for _, e := range entries {
		name := e.Name()
		if path.Ext(name) != ".sql" {
			t.Errorf("%s: not a .sql file", name)
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			t.Errorf("%s: name must be NNNNN_description.sql", name)
			continue
		}
		v, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil || v <= 0 {
			t.Errorf("%s: version prefix %q is not a positive integer", name, prefix)
			continue
		}
		if v <= last {
			t.Errorf("%s: version %d is not greater than the previous %d", name, v, last)
		}
		last = v
		body, err := fs.ReadFile(Migrations(), name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "-- +goose Up") {
			t.Errorf("%s: missing the goose Up marker", name)
		}
		if strings.Contains(string(body), "-- +goose Down") {
			t.Errorf("%s: migrations are forward-only; remove the Down section", name)
		}
	}
}

func TestRedact(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"cannot parse `postgres://u:s3cret@h:5432/db`", "cannot parse `postgres://u:***@h:5432/db`"},
		{"dial tcp 127.0.0.1:5432: connection refused", "dial tcp 127.0.0.1:5432: connection refused"},
		{"postgresql://froodbase:p%40ss@postgres/froodbase?sslmode=disable", "postgresql://froodbase:***@postgres/froodbase?sslmode=disable"},
	}
	for _, tt := range tests {
		got := redact(errors.New(tt.in)).Error()
		if got != tt.want {
			t.Errorf("redact(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
