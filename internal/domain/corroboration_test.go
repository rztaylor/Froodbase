package domain

import "testing"

func ev(id, root ID) Evidence {
	return Evidence{ID: id, SourceKind: SourceDocument, SourceURI: "https://example.com/" + string(id), AccessClass: AccessArea, RootEvidenceID: root, RecordedAt: t1}
}

// A report, an excerpt from it and a summary of both are one line of evidence (PR-12, TR-25).
func TestCorroboration(t *testing.T) {
	tests := []struct {
		name     string
		evidence []Evidence
		want     int
	}{
		{"none", nil, 0},
		{"one root", []Evidence{ev("report", "")}, 1},
		{"three sharing one root", []Evidence{ev("report", ""), ev("excerpt", "report"), ev("summary", "report")}, 1},
		{"restatements only, root absent from the set", []Evidence{ev("excerpt", "report"), ev("summary", "report")}, 1},
		{"two roots", []Evidence{ev("report", ""), ev("excerpt", "report"), ev("ticket", "")}, 2},
		{"chain resolves to the original", []Evidence{ev("report", ""), ev("excerpt", "report"), ev("summary", "excerpt")}, 1},
		{"same record twice", []Evidence{ev("report", ""), ev("report", "")}, 1},
		{"cycle counts once", []Evidence{ev("a", "b"), ev("b", "a")}, 1},
		{"lead-in to a cycle counts once", []Evidence{ev("0", "a"), ev("a", "b"), ev("b", "a")}, 1},
		{"lead-in from the other side", []Evidence{ev("z", "b"), ev("a", "b"), ev("b", "a")}, 1},
		{"independent", []Evidence{ev("a", ""), ev("b", ""), ev("c", "")}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Corroboration(tt.evidence); got != tt.want {
				t.Errorf("Corroboration() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEvidenceRoot(t *testing.T) {
	if got := ev("report", "").Root(); got != "report" {
		t.Errorf("Root() of an original = %q, want its own id", got)
	}
	if got := ev("excerpt", "report").Root(); got != "report" {
		t.Errorf("Root() of a restatement = %q, want report", got)
	}
}
