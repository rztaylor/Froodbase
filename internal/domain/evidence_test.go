package domain

import (
	"errors"
	"testing"
)

func shared(kind SourceKind, uri string) Evidence {
	return Evidence{ID: "ev1", SourceKind: kind, SourceURI: uri, SourceRevision: "r1", Excerpt: "...", AccessClass: AccessArea, RecordedAt: t1}
}

func TestEvidenceValidate(t *testing.T) {
	tests := []struct {
		name string
		e    Evidence
		want error
	}{
		// Documented shared kinds and URI patterns (TR-24, TR-61).
		{"repository", shared(SourceRepository, "https://github.com/rztaylor/Froodbase/blob/main/README.md"), nil},
		{"repository on azure devops", shared(SourceRepository, "https://dev.azure.com/org/project/_git/repo?path=/a.go"), nil},
		{"ticket", shared(SourceTicket, "https://dev.azure.com/org/project/_workitems/edit/123"), nil},
		{"telemetry query", shared(SourceTelemetryQuery, "https://portal.azure.com/#blade/Microsoft_Azure_Monitoring_Logs/LogsBlade/query/abc"), nil},
		{"document", shared(SourceDocument, "https://contoso.sharepoint.com/sites/eng/doc.docx"), nil},
		{"report revision", shared(SourceReportRevision, "froodbase://report-revisions/7f3a"), nil},

		// Local sources are not shareable citations (PR-11).
		{"file scheme", shared(SourceDocument, "file:///Users/robert/reports/incident.md"), ErrLocalSource},
		{"absolute path", shared(SourceDocument, "/Users/robert/reports/incident.md"), ErrLocalSource},
		{"relative path", shared(SourceDocument, "./reports/incident.md"), ErrLocalSource},
		{"home path", shared(SourceDocument, "~/reports/incident.md"), ErrLocalSource},
		{"windows drive", shared(SourceDocument, `C:\Users\robert\incident.md`), ErrLocalSource},
		{"unc path", shared(SourceDocument, `\\server\share\incident.md`), ErrLocalSource},
		{"localhost", shared(SourceTelemetryQuery, "https://localhost:8443/query/1"), ErrLocalSource},
		{"loopback", shared(SourceTelemetryQuery, "https://127.0.0.1/query/1"), ErrLocalSource},

		// Wrong pattern for the kind.
		{"http not https", shared(SourceRepository, "http://github.com/rztaylor/Froodbase"), ErrURINotAllowed},
		{"ftp", shared(SourceDocument, "ftp://files.example.com/doc.pdf"), ErrURINotAllowed},
		{"bare host", shared(SourceDocument, "https://"), ErrURINotAllowed},
		{"report revision as https", shared(SourceReportRevision, "https://example.com/report/1"), ErrURINotAllowed},
		{"froodbase uri for a ticket", shared(SourceTicket, "froodbase://report-revisions/7f3a"), ErrURINotAllowed},
		{"report revision without id", shared(SourceReportRevision, "froodbase://report-revisions/"), ErrURINotAllowed},
		{"froodbase wrong collection", shared(SourceReportRevision, "froodbase://entities/7f3a"), ErrURINotAllowed},
		{"empty uri", shared(SourceDocument, ""), ErrURINotAllowed},
		{"unparseable", shared(SourceDocument, "https://exa mple.com/%zz"), ErrURINotAllowed},

		// Unknown kinds and missing fields.
		{"unknown kind", shared("email", "https://mail.example.com/1"), ErrUnknownSourceKind},
		{"empty kind", shared("", "https://example.com/1"), ErrUnknownSourceKind},
		{"unknown access class", func() Evidence {
			e := shared(SourceDocument, "https://example.com/doc")
			e.AccessClass = "public"
			return e
		}(), ErrUnknownAccessClass},
		{"no revision", func() Evidence {
			e := shared(SourceTicket, "https://dev.azure.com/org/project/_workitems/edit/123")
			e.SourceRevision = " "
			return e
		}(), ErrNoRevision},
		{"report revision needs no separate revision", func() Evidence {
			e := shared(SourceReportRevision, "froodbase://report-revisions/7f3a")
			e.SourceRevision = ""
			return e
		}(), nil},
		{"empty id", func() Evidence {
			e := shared(SourceDocument, "https://example.com/doc")
			e.ID = ""
			return e
		}(), ErrEmptyID},
		{"no recorded_at", func() Evidence {
			e := shared(SourceDocument, "https://example.com/doc")
			e.RecordedAt = zeroTime()
			return e
		}(), ErrNoRecordedAt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.e.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestParseSourceKind(t *testing.T) {
	kinds := SourceKinds()
	if len(kinds) != 5 {
		t.Fatalf("SourceKinds() = %v, want five documented kinds", kinds)
	}
	for _, k := range kinds {
		if got, err := ParseSourceKind(string(k)); err != nil || got != k {
			t.Errorf("ParseSourceKind(%q) = %q, %v", k, got, err)
		}
	}
	for _, s := range []string{"", "Repository", "email", "report"} {
		if _, err := ParseSourceKind(s); !errors.Is(err, ErrUnknownSourceKind) {
			t.Errorf("ParseSourceKind(%q) = %v, want ErrUnknownSourceKind", s, err)
		}
	}
}

// The access class of evidence may not be more permissive than its source's (TR-61).
func TestEvidenceCheckAccess(t *testing.T) {
	tests := []struct {
		evidence, source AccessClass
		want             error
	}{
		{AccessRestricted, AccessRestricted, nil},
		{AccessRestricted, AccessArea, nil},
		{AccessRestricted, AccessOrganisation, nil},
		{AccessArea, AccessArea, nil},
		{AccessArea, AccessOrganisation, nil},
		{AccessOrganisation, AccessOrganisation, nil},
		{AccessArea, AccessRestricted, ErrAccessTooPermissive},
		{AccessOrganisation, AccessRestricted, ErrAccessTooPermissive},
		{AccessOrganisation, AccessArea, ErrAccessTooPermissive},
		{"public", AccessOrganisation, ErrUnknownAccessClass},
		{AccessArea, "public", ErrUnknownAccessClass},
	}
	for _, tt := range tests {
		e := shared(SourceDocument, "https://example.com/doc")
		e.AccessClass = tt.evidence
		if err := e.CheckAccess(tt.source); !errors.Is(err, tt.want) {
			t.Errorf("CheckAccess(evidence=%q, source=%q) = %v, want %v", tt.evidence, tt.source, err, tt.want)
		}
	}
}

func TestAccessClassMorePermissiveThan(t *testing.T) {
	tests := []struct {
		a, b AccessClass
		want bool
	}{
		{AccessRestricted, AccessArea, false},
		{AccessArea, AccessRestricted, true},
		{AccessOrganisation, AccessArea, true},
		{AccessArea, AccessArea, false},
	}
	for _, tt := range tests {
		if got := tt.a.MorePermissiveThan(tt.b); got != tt.want {
			t.Errorf("%q.MorePermissiveThan(%q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
	for _, c := range []AccessClass{AccessRestricted, AccessArea, AccessOrganisation} {
		if got, err := ParseAccessClass(string(c)); err != nil || got != c {
			t.Errorf("ParseAccessClass(%q) = %q, %v", c, got, err)
		}
	}
	for _, s := range []string{"", "public", "Area"} {
		if _, err := ParseAccessClass(s); !errors.Is(err, ErrUnknownAccessClass) {
			t.Errorf("ParseAccessClass(%q) = %v, want ErrUnknownAccessClass", s, err)
		}
	}
}
