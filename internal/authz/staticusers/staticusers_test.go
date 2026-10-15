package staticusers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rztaylor/froodbase/internal/authz"
)

// hashOf is bcrypt("secret", cost 4), cheap for tests.
const hashOf = "$2a$04$pG5Lq4Zs0z2Y2y7d8eJtOeH9q3O3H6mU0h6zQvEZk9sYt8iZ4Jf1u"

func validFile(t *testing.T) []byte {
	t.Helper()
	h, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	return []byte(`{"users":[
	  {"username":"alice","display_name":"Alice","password_hash":"` + h + `","grants":{"support":["reviewer"],"arch":["reader"]}},
	  {"username":"bob","password_hash":"` + h + `","grants":{}}
	]}`)
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{"not json", `{users:`, "invalid JSON"},
		{"unknown field", `{"users":[{"username":"a","password_hash":"` + hashOf + `","password":"x"}]}`, "invalid JSON"},
		{"no users", `{"users":[]}`, "no users"},
		{"empty username", `{"users":[{"username":"","password_hash":"` + hashOf + `"}]}`, "username is empty"},
		{"duplicate", `{"users":[{"username":"a","password_hash":"` + hashOf + `"},{"username":"a","password_hash":"` + hashOf + `"}]}`, "defined twice"},
		{"plaintext password", `{"users":[{"username":"a","password_hash":"hunter2"}]}`, "not a bcrypt hash"},
		{"empty hash", `{"users":[{"username":"a","password_hash":""}]}`, "password_hash is empty"},
		{"garbled hash", `{"users":[{"username":"a","password_hash":"$2a$zz$notreal"}]}`, "not a valid bcrypt hash"},
		{"unknown role", `{"users":[{"username":"a","password_hash":"` + hashOf + `","grants":{"x":["owner"]}}]}`, "unknown role"},
		{"empty area", `{"users":[{"username":"a","password_hash":"` + hashOf + `","grants":{"":["reader"]}}]}`, "empty area"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.data))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "hunter2") {
				t.Fatal("error echoes the plaintext value")
			}
		})
	}

	s, err := Parse(validFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", s.Len())
	}
}

func TestLoad(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file accepted")
	}
	path := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(path, validFile(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticateAndBuild(t *testing.T) {
	s, err := Parse(validFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Authenticate("alice", "wrong"); ok {
		t.Error("wrong password accepted")
	}
	if _, ok := s.Authenticate("nobody", "secret"); ok {
		t.Error("unknown user accepted")
	}
	if _, ok := s.Authenticate("alice", ""); ok {
		t.Error("empty password accepted")
	}
	p, ok := s.Authenticate("alice", "secret")
	if !ok || p.Subject != "alice" {
		t.Fatalf("Authenticate(alice) = %+v, %v", p, ok)
	}

	ac, err := s.Build(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if ac.UserID != "alice" || ac.DisplayName != "Alice" {
		t.Errorf("context = %+v", ac)
	}
	if !ac.Allows("support", authz.RoleContributor) || ac.Allows("arch", authz.RoleContributor) {
		t.Errorf("grants wrong: %+v", ac.Grants)
	}
	ac.Grants["support"][0] = authz.RoleReader
	again, _ := s.Build(context.Background(), p)
	if again.Grants["support"][0] != authz.RoleReviewer {
		t.Error("Build returned shared grant slices")
	}

	bob, err := s.Build(context.Background(), authz.Principal{Subject: "bob"})
	if err != nil || bob.DisplayName != "bob" || len(bob.Grants) != 0 {
		t.Errorf("bob = %+v, %v", bob, err)
	}
	if _, err := s.Build(context.Background(), authz.Principal{Subject: "nobody"}); err == nil {
		t.Error("Build accepted an unknown subject")
	}
}

func TestHashPassword(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Error("empty password hashed")
	}
	h, err := HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkHash(h); err != nil {
		t.Errorf("generated hash rejected: %v", err)
	}
}
