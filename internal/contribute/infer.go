package contribute

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/wire"
)

// inferInput returns the change set with what the server can safely work
// out filled in, so a caller need not spell it out (FRB-51):
//
//   - a record without a ref gets one from its list and position
//     (entity-1, evidence-2, assertion-1, observation-3), never one the
//     caller used;
//   - evidence without a source kind takes the kind its URI shows
//     (inferSourceKind);
//   - evidence without an access class is area;
//   - repository evidence without a revision takes the commit its URL pins
//     (pinnedRevision).
//
// What it cannot infer stays required: evidence whose kind the URI does not
// show, and https evidence with no revision, are refused here with a
// message that says what to pass. Command and statement revisions are left
// to domain.Evidence.WithDefaults. The caller's slices are never written.
// Domain validation is unchanged and still strict.
func inferInput(in wire.ContributeInput) (wire.ContributeInput, error) {
	in.Entities = append([]wire.EntityInput(nil), in.Entities...)
	in.Evidence = append([]wire.EvidenceInput(nil), in.Evidence...)
	in.Assertions = append([]wire.AssertionInput(nil), in.Assertions...)
	in.Observations = append([]wire.ObservationInput(nil), in.Observations...)

	refs := newRefMinter(in)
	for i := range in.Entities {
		refs.fill(&in.Entities[i].Ref, "entity", i)
	}
	for i := range in.Evidence {
		ev := &in.Evidence[i]
		refs.fill(&ev.Ref, "evidence", i)
		if err := inferEvidence(ev); err != nil {
			return wire.ContributeInput{}, err
		}
	}
	for i := range in.Assertions {
		refs.fill(&in.Assertions[i].Ref, "assertion", i)
	}
	for i := range in.Observations {
		refs.fill(&in.Observations[i].Ref, "observation", i)
	}
	return in, nil
}

// inferEvidence fills one evidence record's kind, access class and
// revision where they can be told from the rest of it.
func inferEvidence(ev *wire.EvidenceInput) error {
	ev.SourceKind = strings.TrimSpace(ev.SourceKind)
	if ev.SourceKind == "" {
		kind, ok := inferSourceKind(ev.SourceURI)
		if !ok {
			return fmt.Errorf("%w: evidence %q: source_kind is required: %s; say which of repository, ticket, telemetry_query, document, command (a command you ran) or statement (something the person you act for knows)",
				ErrInvalidInput, ev.Ref, unrecognised(ev.SourceURI))
		}
		ev.SourceKind = string(kind)
	}
	if strings.TrimSpace(ev.AccessClass) == "" {
		ev.AccessClass = string(domain.AccessArea)
	}
	if strings.TrimSpace(ev.SourceRevision) != "" {
		return nil
	}
	switch kind := domain.SourceKind(ev.SourceKind); {
	case kind == domain.SourceCommand || kind == domain.SourceStatement:
		return nil // the revision is when it ran or was said (domain.Evidence.WithDefaults)
	case !kind.Valid():
		return nil // the domain refuses an unknown kind with its own message
	case kind == domain.SourceRepository:
		if rev, ok := pinnedRevision(ev.SourceURI); ok {
			ev.SourceRevision = rev
			return nil
		}
		return fmt.Errorf("%w: evidence %q: source_revision is required: the URL does not pin a commit; pass the commit SHA you read, or cite a permalink with the SHA in it", ErrInvalidInput, ev.Ref)
	}
	return fmt.Errorf("%w: evidence %q: source_revision is required for %s evidence: pass the revision you read (a work-item revision, query snapshot or document version, or the date you read it)", ErrInvalidInput, ev.Ref, ev.SourceKind)
}

func unrecognised(uri string) string {
	if strings.TrimSpace(uri) == "" {
		return "there is no source_uri to tell it from"
	}
	return "the source_uri is not a kind of link Froodbase recognises"
}

// inferSourceKind returns the source kind a URI shows, or false when it
// cannot tell. Only well-known forms are recognised: the command schemes
// (azure, k8s, sql, host) and person:// for a statement; on https, work
// items, Jira issues (/browse/KEY-1) and GitHub issues are tickets; GitHub,
// GitLab and Bitbucket blob, tree and commit URLs and Azure DevOps
// repositories are repositories; Confluence, SharePoint and Azure DevOps
// wiki pages are documents. Telemetry queries are never inferred.
func inferSourceKind(raw string) (domain.SourceKind, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "azure", "k8s", "sql", "host":
		return domain.SourceCommand, true
	case "person":
		return domain.SourceStatement, true
	case "https":
	default:
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	segs := segments(u.Path)
	devops := host == "dev.azure.com" || strings.HasSuffix(host, ".visualstudio.com")
	switch {
	case devops && has(segs, "_workitems"):
		return domain.SourceTicket, true
	case devops && has(segs, "_git"):
		return domain.SourceRepository, true
	case devops && has(segs, "_wiki"):
		return domain.SourceDocument, true
	case jiraIssue(segs):
		return domain.SourceTicket, true
	case host == "github.com" && len(segs) >= 4 && segs[2] == "issues":
		return domain.SourceTicket, true
	case host == "github.com" && len(segs) >= 4 && (segs[2] == "blob" || segs[2] == "tree" || segs[2] == "commit"):
		return domain.SourceRepository, true
	case gitlabRef(segs) >= 0:
		return domain.SourceRepository, true
	case host == "bitbucket.org" && len(segs) >= 4 && segs[2] == "src":
		return domain.SourceRepository, true
	case strings.HasSuffix(host, ".atlassian.net") && len(segs) > 0 && segs[0] == "wiki":
		return domain.SourceDocument, true
	case hostLabel(host, "confluence"):
		return domain.SourceDocument, true
	case host == "sharepoint.com" || strings.HasSuffix(host, ".sharepoint.com"):
		return domain.SourceDocument, true
	}
	return "", false
}

var (
	jiraKey   = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-[0-9]+$`)
	commitSHA = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)
)

// jiraIssue reports whether the path is Jira's /browse/<KEY-123>, on any
// host (Jira Cloud or self-hosted).
func jiraIssue(segs []string) bool {
	for i := 0; i+1 < len(segs); i++ {
		if segs[i] == "browse" && jiraKey.MatchString(segs[i+1]) {
			return true
		}
	}
	return false
}

// gitlabRef returns the index of the ref segment of a GitLab
// /-/blob|tree|commit/<ref> path, on any host, or -1.
func gitlabRef(segs []string) int {
	for i := 0; i+2 < len(segs); i++ {
		if segs[i] == "-" && (segs[i+1] == "blob" || segs[i+1] == "tree" || segs[i+1] == "commit") {
			return i + 2
		}
	}
	return -1
}

// pinnedRevision returns the full commit SHA (40 or 64 hex digits, lower
// case) a repository URL pins, or false when it names a branch, a short
// SHA or no ref: GitHub /blob|tree|commit/<sha>, GitLab /-/blob/<sha>,
// Bitbucket /src/<sha>, and Azure DevOps version=GC<sha>.
func pinnedRevision(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	segs := segments(u.Path)
	ref := ""
	switch {
	case host == "dev.azure.com" || strings.HasSuffix(host, ".visualstudio.com"):
		if v := u.Query().Get("version"); has(segs, "_git") && strings.HasPrefix(v, "GC") {
			ref = v[2:]
		}
	case host == "github.com" && len(segs) >= 4 && (segs[2] == "blob" || segs[2] == "tree" || segs[2] == "commit"):
		ref = segs[3]
	case host == "bitbucket.org" && len(segs) >= 4 && segs[2] == "src":
		ref = segs[3]
	default:
		if i := gitlabRef(segs); i >= 0 {
			ref = segs[i]
		}
	}
	if !commitSHA.MatchString(ref) {
		return "", false
	}
	return strings.ToLower(ref), true
}

func segments(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func has(segs []string, s string) bool {
	for _, x := range segs {
		if x == s {
			return true
		}
	}
	return false
}

// hostLabel reports whether one dot-separated label of host is label.
func hostLabel(host, label string) bool {
	for _, l := range strings.Split(host, ".") {
		if l == label {
			return true
		}
	}
	return false
}

// refMinter generates refs for records that have none, never repeating one
// the caller used.
type refMinter struct{ taken map[string]bool }

func newRefMinter(in wire.ContributeInput) *refMinter {
	m := &refMinter{taken: map[string]bool{}}
	add := func(r string) {
		if strings.TrimSpace(r) != "" {
			m.taken[r] = true
		}
	}
	for _, e := range in.Entities {
		add(e.Ref)
	}
	for _, e := range in.Evidence {
		add(e.Ref)
	}
	for _, a := range in.Assertions {
		add(a.Ref)
	}
	for _, o := range in.Observations {
		add(o.Ref)
	}
	return m
}

// fill sets *ref to <list>-<position+1> when it is blank, adding a suffix
// in the unlikely case the caller already used that name.
func (m *refMinter) fill(ref *string, list string, i int) {
	if strings.TrimSpace(*ref) != "" {
		return
	}
	base := list + "-" + strconv.Itoa(i+1)
	r := base
	for n := 2; m.taken[r]; n++ {
		r = base + "-" + strconv.Itoa(n)
	}
	m.taken[r] = true
	*ref = r
}
