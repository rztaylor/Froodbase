package contribute

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/wire"
)

const sha = "0123456789abcdef0123456789abcdef01234567"

func TestInferSourceKind(t *testing.T) {
	cases := []struct {
		uri  string
		want domain.SourceKind // empty: cannot tell
	}{
		// Tickets: work items, Jira issues, GitHub issues.
		{"https://dev.azure.com/org/project/_workitems/edit/418", domain.SourceTicket},
		{"https://org.visualstudio.com/project/_workitems/edit/418", domain.SourceTicket},
		{"https://contoso.atlassian.net/browse/IMP-42", domain.SourceTicket},
		{"https://jira.contoso.com/browse/IMP-42", domain.SourceTicket},
		{"https://github.com/org/repo/issues/7", domain.SourceTicket},
		// Repositories: blob, tree and commit URLs, Azure DevOps repos.
		{"https://github.com/org/repo/blob/main/cmd/main.go#L10-L20", domain.SourceRepository},
		{"https://github.com/org/repo/blob/" + sha + "/README.md", domain.SourceRepository},
		{"https://github.com/org/repo/tree/" + sha + "/internal", domain.SourceRepository},
		{"https://github.com/org/repo/commit/" + sha, domain.SourceRepository},
		{"https://gitlab.com/group/repo/-/blob/main/README.md", domain.SourceRepository},
		{"https://git.contoso.com/group/repo/-/blob/" + sha + "/README.md", domain.SourceRepository},
		{"https://bitbucket.org/team/repo/src/" + sha + "/README.md", domain.SourceRepository},
		{"https://dev.azure.com/org/project/_git/repo?path=/src/app.cs&version=GC" + sha, domain.SourceRepository},
		{"https://org.visualstudio.com/project/_git/repo?path=/src/app.cs", domain.SourceRepository},
		// Documents: Confluence, SharePoint, Azure DevOps wikis.
		{"https://contoso.atlassian.net/wiki/spaces/IMP/pages/123/Runbook", domain.SourceDocument},
		{"https://confluence.contoso.com/display/IMP/Runbook", domain.SourceDocument},
		{"https://contoso.sharepoint.com/sites/import/Shared%20Documents/design.docx", domain.SourceDocument},
		{"https://contoso-my.sharepoint.com/personal/bob/Documents/notes.docx", domain.SourceDocument},
		{"https://dev.azure.com/org/project/_wiki/wikis/project.wiki/12/Runbook", domain.SourceDocument},
		// Commands and statements by scheme.
		{"azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg", domain.SourceCommand},
		{"k8s://aks-prod/import/deployment/import-api", domain.SourceCommand},
		{"sql://sql-import.database.windows.net/importdb", domain.SourceCommand},
		{"host://build-agent-7", domain.SourceCommand},
		{"person://bob", domain.SourceStatement},
		// Cannot tell.
		{"https://github.com/org/repo", ""},
		{"https://github.com/org/repo/pull/9", ""},
		{"https://portal.azure.com/#blade/Logs", ""},
		{"https://example.com/browse/not-a-key", ""},
		{"https://contoso.atlassian.net/jira/dashboards", ""},
		{"http://dev.azure.com/org/project/_workitems/edit/418", ""},
		{"https://evil.example.com/sharepoint.com/page", ""},
		{"", ""},
		{"/home/bob/notes.md", ""},
		{"not a url", ""},
	}
	for _, c := range cases {
		got, ok := inferSourceKind(c.uri)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("inferSourceKind(%q) = %q, %v; want %q", c.uri, got, ok, c.want)
		}
	}
}

func TestPinnedRevision(t *testing.T) {
	sha256 := strings.Repeat("ab", 32)
	cases := []struct{ uri, want string }{
		{"https://github.com/org/repo/blob/" + sha + "/README.md#L3", sha},
		{"https://github.com/org/repo/blob/" + strings.ToUpper(sha) + "/README.md", sha},
		{"https://github.com/org/repo/tree/" + sha, sha},
		{"https://github.com/org/repo/commit/" + sha, sha},
		{"https://github.com/org/repo/blob/" + sha256 + "/README.md", sha256},
		{"https://gitlab.com/group/sub/repo/-/blob/" + sha + "/README.md", sha},
		{"https://bitbucket.org/team/repo/src/" + sha + "/README.md", sha},
		{"https://dev.azure.com/org/project/_git/repo?path=/a.cs&version=GC" + sha, sha},
		// Not pinned: a branch, a short SHA, no ref at all, a SHA elsewhere.
		{"https://github.com/org/repo/blob/main/README.md", ""},
		{"https://github.com/org/repo/blob/0123456/README.md", ""},
		{"https://github.com/org/repo/blob/" + sha[:39] + "/README.md", ""},
		{"https://github.com/" + sha + "/repo/blob/main/README.md", ""},
		{"https://dev.azure.com/org/project/_git/repo?path=/a.cs&version=GBmain", ""},
		{"https://dev.azure.com/org/project/_git/repo?path=/" + sha, ""},
		{"https://dev.azure.com/org/project/_workitems/edit/418", ""},
	}
	for _, c := range cases {
		got, ok := pinnedRevision(c.uri)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("pinnedRevision(%q) = %q, %v; want %q", c.uri, got, ok, c.want)
		}
	}
}

// plain is a change set with only what a person's plain request gives:
// no refs on what nothing refers to, no source kinds, access classes or
// revisions the URLs already say.
func plain() wire.ContributeInput {
	ran := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	return wire.ContributeInput{
		Area: "demo", IdempotencyKey: "k1",
		Entities: []wire.EntityInput{{Ref: "tool", Type: "service", Names: []wire.Name{{Name: "Import Tool"}}}},
		Evidence: []wire.EvidenceInput{
			{Ref: "wi", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/418", SourceRevision: "7"},
			{Ref: "code", SourceURI: "https://github.com/org/repo/blob/" + sha + "/retry.go#L10-L20"},
			{Ref: "run", SourceURI: "k8s://aks-prod/import/deployment/import-api", Locator: "kubectl -n import get deployment import-api -o jsonpath={.spec.replicas}", Excerpt: "3", ObservedAt: &ran},
			{SourceKind: "document", SourceURI: "https://contoso.sharepoint.com/sites/import/design.docx", SourceRevision: "v4", AccessClass: "restricted"},
		},
		Assertions: []wire.AssertionInput{
			{Ref: "seen", Kind: "observation", Subject: "tool", BodyMarkdown: "Callbacks fail on 2.3", Evidence: []string{"wi"}},
			{Kind: "inference", Subject: "tool", BodyMarkdown: "The retry change causes it", DerivedFrom: []string{"seen"}, Evidence: []string{"code"}},
		},
		Observations: []wire.ObservationInput{{Subject: "tool", BodyMarkdown: "Three replicas run in prod", Evidence: []string{"run"}}},
	}
}

func TestInferInputFillsWhatItCan(t *testing.T) {
	in := plain()
	original := plain()
	got, err := inferInput(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, original) {
		t.Error("inferInput changed the caller's input")
	}
	wantEvidence := []struct{ ref, kind, revision, access string }{
		{"wi", "ticket", "7", "area"},
		{"code", "repository", sha, "area"},
		{"run", "command", "", "area"}, // the domain defaults a command's revision to observed_at
		{"evidence-4", "document", "v4", "restricted"},
	}
	for i, w := range wantEvidence {
		ev := got.Evidence[i]
		if ev.Ref != w.ref || ev.SourceKind != w.kind || ev.SourceRevision != w.revision || ev.AccessClass != w.access {
			t.Errorf("evidence %d = ref %q kind %q revision %q access %q; want %+v", i, ev.Ref, ev.SourceKind, ev.SourceRevision, ev.AccessClass, w)
		}
	}
	if r := got.Assertions[1].Ref; r != "assertion-2" {
		t.Errorf("second assertion ref = %q, want assertion-2", r)
	}
	if r := got.Observations[0].Ref; r != "observation-1" {
		t.Errorf("observation ref = %q, want observation-1", r)
	}
	if err := validateInput(got); err != nil {
		t.Errorf("inferred input does not validate: %v", err)
	}
}

func TestInferInputRefsAvoidCallerRefs(t *testing.T) {
	in := wire.ContributeInput{
		Area: "demo", IdempotencyKey: "k",
		Entities: []wire.EntityInput{
			{Type: "service", Names: []wire.Name{{Name: "A"}}},
			{Ref: "entity-1", Type: "service", Names: []wire.Name{{Name: "B"}}},
			{Ref: " ", Type: "service", Names: []wire.Name{{Name: "C"}}},
		},
	}
	got, err := inferInput(in)
	if err != nil {
		t.Fatal(err)
	}
	refs := []string{got.Entities[0].Ref, got.Entities[1].Ref, got.Entities[2].Ref}
	if refs[1] != "entity-1" || refs[0] == "entity-1" || refs[2] == "entity-1" || refs[0] == refs[2] || refs[0] == "" || refs[2] == "" {
		t.Errorf("refs = %q, want distinct generated refs that keep the caller's entity-1", refs)
	}
}

func TestInferInputDeterministic(t *testing.T) {
	a, err := inferInput(plain())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := inferInput(plain())
	if requestHash(a) != requestHash(b) {
		t.Error("the same plain request hashes differently, so a retry would be refused")
	}
	explicit := plain()
	explicit.Evidence[0].SourceKind, explicit.Evidence[0].AccessClass = "ticket", "area"
	c, _ := inferInput(explicit)
	if requestHash(a) != requestHash(c) {
		t.Error("spelling out what the server infers changes the request hash")
	}
}

func TestInferInputRefusesWhatItCannotTell(t *testing.T) {
	cases := map[string]struct {
		ev   wire.EvidenceInput
		want string
	}{
		"unknown kind of URL":  {wire.EvidenceInput{SourceURI: "https://example.com/page", SourceRevision: "1"}, "source_kind is required"},
		"no kind and no URI":   {wire.EvidenceInput{Excerpt: "I saw it"}, "source_kind is required"},
		"branch, not a commit": {wire.EvidenceInput{SourceURI: "https://github.com/org/repo/blob/main/retry.go"}, "source_revision is required"},
		"ticket, no revision":  {wire.EvidenceInput{SourceURI: "https://contoso.atlassian.net/browse/IMP-42"}, "source_revision is required"},
		"given kind, no revision": {wire.EvidenceInput{SourceKind: "telemetry_query", SourceURI: "https://portal.azure.com/#blade/Logs"},
			"source_revision is required"},
	}
	for name, c := range cases {
		in := plain()
		in.Evidence = []wire.EvidenceInput{c.ev}
		_, err := inferInput(in)
		if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want ErrInvalidInput with %q", name, err, c.want)
		}
	}
}

func TestInferInputLeavesStatementsAndCommandsToTheDomain(t *testing.T) {
	said := time.Date(2026, 10, 21, 9, 15, 0, 0, time.UTC)
	in := plain()
	in.Evidence = []wire.EvidenceInput{
		{SourceKind: "statement", Excerpt: "We turned HTTP/2 on in September.", ObservedAt: &said},
		{SourceURI: "person://bob", Excerpt: "And off again in October.", ObservedAt: &said},
	}
	got, err := inferInput(in)
	if err != nil {
		t.Fatal(err)
	}
	for i, ev := range got.Evidence {
		if ev.SourceKind != "statement" || ev.SourceRevision != "" || ev.AccessClass != "area" {
			t.Errorf("statement %d = %+v, want kind statement, revision left to observed_at, access area", i, ev)
		}
	}
}
