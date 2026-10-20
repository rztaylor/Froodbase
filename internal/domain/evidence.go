package domain

import (
	"net"
	"net/url"
	"strings"
	"time"
)

// SourceKind names where evidence comes from (TR-24). Only these kinds are
// shareable citations; anything else is rejected (TR-61).
type SourceKind string

// The documented shared source kinds.
const (
	SourceRepository     SourceKind = "repository"
	SourceTicket         SourceKind = "ticket"
	SourceTelemetryQuery SourceKind = "telemetry_query"
	SourceDocument       SourceKind = "document"
)

// SourceKinds lists the documented kinds in a stable order.
func SourceKinds() []SourceKind {
	return []SourceKind{SourceRepository, SourceTicket, SourceTelemetryQuery, SourceDocument}
}

// Valid reports whether k is a documented source kind.
func (k SourceKind) Valid() bool {
	for _, c := range SourceKinds() {
		if c == k {
			return true
		}
	}
	return false
}

// ParseSourceKind returns the kind named by s, or ErrUnknownSourceKind.
func ParseSourceKind(s string) (SourceKind, error) {
	k := SourceKind(s)
	if !k.Valid() {
		return "", ErrUnknownSourceKind
	}
	return k, nil
}

// AccessClass says who may see a piece of evidence, from
// narrowest to widest: the people named on the source, everyone with the
// knowledge area, everyone in the organisation. Evidence never claims a wider
// class than its source (TR-61), so restricted material stays restricted in
// every derived form (PR-83).
type AccessClass string

// The access classes, narrowest first.
const (
	AccessRestricted   AccessClass = "restricted"
	AccessArea         AccessClass = "area"
	AccessOrganisation AccessClass = "organisation"
)

// accessRank orders the classes; a higher rank is more permissive.
var accessRank = map[AccessClass]int{AccessRestricted: 1, AccessArea: 2, AccessOrganisation: 3}

// Valid reports whether c is a known access class.
func (c AccessClass) Valid() bool {
	_, ok := accessRank[c]
	return ok
}

// ParseAccessClass returns the class named by s, or ErrUnknownAccessClass.
func ParseAccessClass(s string) (AccessClass, error) {
	c := AccessClass(s)
	if !c.Valid() {
		return "", ErrUnknownAccessClass
	}
	return c, nil
}

// MorePermissiveThan reports whether c admits a wider audience than other.
// Unknown classes rank below every known one.
func (c AccessClass) MorePermissiveThan(other AccessClass) bool {
	return accessRank[c] > accessRank[other]
}

// Evidence is a reference to a source the audience can open (PR-10, PR-11,
// TR-24): what kind of source, where (URI) and which revision, the excerpt
// relied on, and who may see it. One evidence record can support many claims.
//
// RootEvidenceID points at the original observation when this record restates
// another (a report excerpt quoting a telemetry query, a summary of a report).
// It is empty when the record is itself the original. Corroboration counts
// distinct roots, not records (PR-12, TR-25).
type Evidence struct {
	ID             ID
	SourceKind     SourceKind
	SourceURI      string
	SourceRevision string // commit, work-item revision, query snapshot or document version
	Excerpt        string
	AccessClass    AccessClass
	RootEvidenceID ID
	ObservedAt     *time.Time // when the source showed this, if known
	RecordedAt     time.Time
}

// Root returns the id that corroboration counts for this record: the root it
// restates, or its own id when it is the original.
func (e Evidence) Root() ID {
	if e.RootEvidenceID != "" {
		return e.RootEvidenceID
	}
	return e.ID
}

// Validate checks that the evidence is a shareable citation (TR-61): the
// source kind is documented, the URI matches the allowed pattern for that
// kind, the access class is known, the record names the revision it cites
// (PR-10) and has an id and a recording time. Local file paths, relative and
// home paths, Windows drive and UNC paths, and URLs on the contributor's own
// machine fail with ErrLocalSource; other mismatches fail with
// ErrURINotAllowed.
//
// Allowed pattern: every source kind cites an https URL with a host that is
// not loopback. Froodbase never holds a copy of a source (PR-50), so there is
// no internal URI scheme.
func (e Evidence) Validate() error {
	if err := e.ID.Validate(); err != nil {
		return fieldErr("evidence id", err)
	}
	if !e.SourceKind.Valid() {
		return fieldErr("source kind", ErrUnknownSourceKind)
	}
	if !e.AccessClass.Valid() {
		return fieldErr("access class", ErrUnknownAccessClass)
	}
	if e.RecordedAt.IsZero() {
		return fieldErr("recorded_at", ErrNoRecordedAt)
	}
	if err := validateSourceURI(e.SourceURI); err != nil {
		return fieldErr("source uri", err)
	}
	if strings.TrimSpace(e.SourceRevision) == "" {
		return fieldErr("source revision", ErrNoRevision)
	}
	return nil
}

// CheckAccess reports ErrAccessTooPermissive when the evidence claims a wider
// audience than its source allows (TR-61), and ErrUnknownAccessClass when
// either class is unknown.
func (e Evidence) CheckAccess(source AccessClass) error {
	if !e.AccessClass.Valid() || !source.Valid() {
		return fieldErr("access class", ErrUnknownAccessClass)
	}
	if e.AccessClass.MorePermissiveThan(source) {
		return fieldErr("access class", ErrAccessTooPermissive)
	}
	return nil
}

func validateSourceURI(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ErrURINotAllowed
	}
	if looksLocalPath(raw) {
		return ErrLocalSource
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ErrURINotAllowed
	}
	scheme := strings.ToLower(u.Scheme)
	switch {
	case scheme == "" || scheme == "file":
		return ErrLocalSource
	case len(scheme) == 1: // a Windows drive letter parsed as a scheme
		return ErrLocalSource
	}
	if scheme != "https" || u.Hostname() == "" {
		return ErrURINotAllowed
	}
	if isLoopbackHost(u.Hostname()) {
		return ErrLocalSource
	}
	return nil
}

// looksLocalPath catches path forms that url.Parse would accept or misread:
// absolute and relative POSIX paths, home paths and UNC paths.
func looksLocalPath(s string) bool {
	for _, p := range []string{"/", "./", "../", "~", `\\`, `\`} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func isLoopbackHost(host string) bool {
	h := strings.ToLower(host)
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsUnspecified()
	}
	return false
}
