package staticusers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/rztaylor/froodbase/internal/authz"
)

// File is the on-disk format: {"users": [ ... ]}.
type File struct {
	Users []User `json:"users"`
}

// User is one entry of the users file.
type User struct {
	// Username is the login name and the stable user id.
	Username string `json:"username"`
	// DisplayName is shown in audit and whoami.
	DisplayName string `json:"display_name"`
	// PasswordHash is a bcrypt hash ($2a$, $2b$ or $2y$). Plaintext is refused.
	PasswordHash string `json:"password_hash"`
	// Grants maps a knowledge area id to role names.
	Grants map[string][]string `json:"grants"`
}

// Store is the loaded, validated user list.
type Store struct {
	users map[string]*entry
}

type entry struct {
	hash    []byte
	context authz.Context
}

// dummyHash is compared against when the username is unknown so that the
// response time does not reveal which usernames exist.
var dummyHash = []byte("$2a$10$CwTycUXWue0Thq9StjUM0uJ8ZV3Hc8m1z1z0x8lP1kG1t9dN1Yq7e")

// Load reads and validates the users file at path.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("staticusers: reading %s: %w", path, err)
	}
	s, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("staticusers: %s: %w", path, err)
	}
	return s, nil
}

// Parse validates the JSON users file contents.
func Parse(data []byte) (*Store, error) {
	var f File
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if len(f.Users) == 0 {
		return nil, errors.New("no users defined")
	}
	s := &Store{users: make(map[string]*entry, len(f.Users))}
	for i, u := range f.Users {
		if u.Username == "" {
			return nil, fmt.Errorf("user %d: username is empty", i+1)
		}
		if _, dup := s.users[u.Username]; dup {
			return nil, fmt.Errorf("user %q: defined twice", u.Username)
		}
		if err := checkHash(u.PasswordHash); err != nil {
			return nil, fmt.Errorf("user %q: %w", u.Username, err)
		}
		grants := make(map[string][]authz.Role, len(u.Grants))
		for area, roles := range u.Grants {
			if area == "" {
				return nil, fmt.Errorf("user %q: empty area name", u.Username)
			}
			for _, r := range roles {
				role, err := authz.ParseRole(r)
				if err != nil {
					return nil, fmt.Errorf("user %q, area %q: %w", u.Username, area, err)
				}
				grants[area] = append(grants[area], role)
			}
		}
		name := u.DisplayName
		if name == "" {
			name = u.Username
		}
		s.users[u.Username] = &entry{
			hash:    []byte(u.PasswordHash),
			context: authz.Context{UserID: u.Username, DisplayName: name, Grants: grants},
		}
	}
	return s, nil
}

// checkHash accepts bcrypt hashes only. Anything else, including a plaintext
// password, is an error; the value is never echoed.
func checkHash(h string) error {
	if h == "" {
		return errors.New("password_hash is empty")
	}
	if !strings.HasPrefix(h, "$2a$") && !strings.HasPrefix(h, "$2b$") && !strings.HasPrefix(h, "$2y$") {
		return errors.New("password_hash is not a bcrypt hash (generate one with: froodbase-api hash-password)")
	}
	if _, err := bcrypt.Cost([]byte(h)); err != nil {
		return errors.New("password_hash is not a valid bcrypt hash")
	}
	return nil
}

// Authenticate checks a username and password and returns the principal on
// success. It takes the same time whether or not the username exists.
func (s *Store) Authenticate(username, password string) (authz.Principal, bool) {
	e, known := s.users[username]
	hash := dummyHash
	if known {
		hash = e.hash
	}
	err := bcrypt.CompareHashAndPassword(hash, []byte(password))
	// subtle.ConstantTimeSelect keeps the final decision branch-free.
	ok := subtle.ConstantTimeSelect(boolToInt(known && err == nil), 1, 0) == 1
	if !ok {
		return authz.Principal{}, false
	}
	return authz.Principal{Subject: username}, true
}

// Build implements authz.Builder from the user list.
func (s *Store) Build(_ context.Context, p authz.Principal) (*authz.Context, error) {
	e, ok := s.users[p.Subject]
	if !ok {
		return nil, fmt.Errorf("staticusers: unknown user %q", p.Subject)
	}
	ac := e.context
	grants := make(map[string][]authz.Role, len(ac.Grants))
	for area, roles := range ac.Grants {
		grants[area] = append([]authz.Role(nil), roles...)
	}
	ac.Grants = grants
	return &ac, nil
}

// Len reports the number of users.
func (s *Store) Len() int {
	return len(s.users)
}

// HashPassword returns a bcrypt hash suitable for password_hash.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("staticusers: password is empty")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("staticusers: hashing password: %w", err)
	}
	return string(h), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
