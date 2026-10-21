package domain

import (
	"net"
	"net/url"
	"strings"
	"time"
)

// SourceKind names where evidence comes from (TR-24). Only these kinds are
// citations the audience can check; anything else is rejected (TR-61).
type SourceKind string

// The documented source kinds. The first four cite an https source at a
// revision. A command observation cites what a command or query observed
// (a resource, a Kubernetes object, a database, a host), the command it ran
// and when; each run is its own root. A statement cites the named person who
// said it and when, with what they said as its excerpt.
const (
	SourceRepository     SourceKind = "repository"
	SourceTicket         SourceKind = "ticket"
	SourceTelemetryQuery SourceKind = "telemetry_query"
	SourceDocument       SourceKind = "document"
	SourceCommand        SourceKind = "command"
	SourceStatement      SourceKind = "statement"
)

// SourceKinds lists the documented kinds in a stable order.
func SourceKinds() []SourceKind {
	return []SourceKind{SourceRepository, SourceTicket, SourceTelemetryQuery, SourceDocument, SourceCommand, SourceStatement}
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

// Evidence is a reference to a source the audience can check (PR-10, PR-11,
// TR-24): what kind of source, where (URI) and which revision, the excerpt
// relied on, and who may see it. One evidence record can support many claims.
// A command observation's locator is the command it ran and its revision the
// time it ran; a statement's excerpt is what was said and its revision the
// time it was said (see WithDefaults).
//
// RootEvidenceID points at the original observation when this record restates
// another (a report excerpt quoting a telemetry query, a summary of a report).
// It is empty when the record is itself the original. Corroboration counts
// distinct roots, not records (PR-12, TR-25).
type Evidence struct {
	ID             ID
	AreaID         ID
	SourceKind     SourceKind
	SourceURI      string
	SourceRevision string // commit, work-item revision, query snapshot or document version
	SourceLocator  string // line range, section or query window inside the source; for a command observation, the command
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

// Validate checks that the evidence is a citation the audience can check
// (TR-61): it belongs to an area, the source kind is documented, the URI
// matches the allowed pattern for that kind, nothing in the URI, locator or
// excerpt looks like a secret (ErrLooksLikeSecret, naming the field and the
// kind of secret, never the value), the access class is known, the record
// names the revision it cites (PR-10) and has an id and a recording time. A
// command observation also needs its command (ErrNoCommand) and the time it
// ran, a statement what was said (ErrNoStatement) and when (ErrNoObservedAt).
// Local file paths, relative and home paths, Windows drive and UNC paths,
// and sources on the contributor's own machine fail with ErrLocalSource;
// other mismatches fail with ErrURINotAllowed.
//
// Allowed patterns: repository, ticket, telemetry_query and document cite an
// https URL with a host that is not loopback. A command observation cites
// azure:///<ARM id> (from /subscriptions/ or /providers/),
// k8s://<cluster>/<namespace>/<kind>/<name> (or <kind>/<name> for a
// cluster-scoped object), sql://<server>/<database> or host://<name>. A
// statement cites person://<user id>. Froodbase never holds a copy of a
// source (PR-50), so there is no internal URI scheme.
func (e Evidence) Validate() error {
	if err := e.ID.Validate(); err != nil {
		return fieldErr("evidence id", err)
	}
	if err := e.AreaID.Validate(); err != nil {
		return fieldErr("area id", err)
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
	for _, f := range []struct{ name, value string }{{"source uri", e.SourceURI}, {"locator", e.SourceLocator}, {"excerpt", e.Excerpt}} {
		if err := checkSecret(f.value); err != nil {
			return fieldErr(f.name, err)
		}
	}
	if err := validateSourceURI(e.SourceKind, e.SourceURI); err != nil {
		return fieldErr("source uri", err)
	}
	switch e.SourceKind {
	case SourceCommand:
		if strings.TrimSpace(e.SourceLocator) == "" {
			return fieldErr("locator", ErrNoCommand)
		}
	case SourceStatement:
		if strings.TrimSpace(e.Excerpt) == "" {
			return fieldErr("excerpt", ErrNoStatement)
		}
	}
	if e.SourceKind.observedAtRequired() && (e.ObservedAt == nil || e.ObservedAt.IsZero()) {
		return fieldErr("observed_at", ErrNoObservedAt)
	}
	if strings.TrimSpace(e.SourceRevision) == "" {
		return fieldErr("source revision", ErrNoRevision)
	}
	return nil
}

// observedAtRequired reports whether the kind's revision is the time it was
// observed: a command run or a statement has no other snapshot.
func (k SourceKind) observedAtRequired() bool {
	return k == SourceCommand || k == SourceStatement
}

// WithDefaults returns e with what can be filled in from the rest of it: a
// command observation or a statement without a revision takes the time it
// ran or was made (RFC 3339, UTC), so running a command again later, or
// saying something again, is a separate record and a separate root.
func (e Evidence) WithDefaults() Evidence {
	if e.SourceKind.observedAtRequired() && strings.TrimSpace(e.SourceRevision) == "" && e.ObservedAt != nil && !e.ObservedAt.IsZero() {
		e.SourceRevision = e.ObservedAt.UTC().Format(time.RFC3339Nano)
	}
	return e
}

// StatementURI is the source URI of a statement made by the user with id userID.
func StatementURI(userID string) string {
	return personScheme + "://" + userID
}

// StatementPerson returns the user id a statement's source URI names, or
// ErrURINotAllowed when it is not person://<user id>.
func StatementPerson(uri string) (ID, error) {
	prefix := personScheme + "://"
	if len(uri) < len(prefix) || !strings.EqualFold(uri[:len(prefix)], prefix) {
		return "", ErrURINotAllowed
	}
	id := uri[len(prefix):]
	if id == "" || strings.ContainsAny(id, "/?#@ \t\r\n") {
		return "", ErrURINotAllowed
	}
	return ID(id), nil
}

const personScheme = "person"

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

func validateSourceURI(kind SourceKind, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ErrURINotAllowed
	}
	if looksLocalPath(raw) {
		return ErrLocalSource
	}
	if kind == SourceStatement {
		_, err := StatementPerson(raw)
		return err
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
	if kind == SourceCommand {
		return validateCommandTarget(scheme, u)
	}
	if scheme != "https" || u.Hostname() == "" {
		return ErrURINotAllowed
	}
	if isLoopbackHost(u.Hostname()) {
		return ErrLocalSource
	}
	return nil
}

// validateCommandTarget checks the provider forms a command observation may
// cite: azure:///<ARM id>, k8s://<cluster>/[<namespace>/]<kind>/<name>,
// sql://<server>/<database> and host://<name>, none with credentials, a
// query or a fragment.
func validateCommandTarget(scheme string, u *url.URL) error {
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return ErrURINotAllowed
	}
	segs, ok := pathSegments(u.Path)
	if !ok {
		return ErrURINotAllowed
	}
	if scheme == "azure" {
		if u.Host != "" || len(segs) < 2 || !strings.EqualFold(segs[0], "subscriptions") && !strings.EqualFold(segs[0], "providers") {
			return ErrURINotAllowed
		}
		return nil
	}
	var want func(int) bool
	switch scheme {
	case "k8s":
		want = func(n int) bool { return n == 2 || n == 3 }
	case "sql":
		want = func(n int) bool { return n == 1 }
	case "host":
		want = func(n int) bool { return n == 0 }
	default:
		return ErrURINotAllowed
	}
	if u.Hostname() == "" || !want(len(segs)) {
		return ErrURINotAllowed
	}
	if isLoopbackHost(u.Hostname()) {
		return ErrLocalSource
	}
	return nil
}

// pathSegments splits a URL path into its segments, reporting false for an
// empty segment between two others.
func pathSegments(p string) ([]string, bool) {
	p = strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/")
	if p == "" {
		return nil, true
	}
	segs := strings.Split(p, "/")
	for _, s := range segs {
		if strings.TrimSpace(s) == "" {
			return nil, false
		}
	}
	return segs, true
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
