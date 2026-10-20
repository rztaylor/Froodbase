package domain

import "testing"

func TestNormaliseQuery(t *testing.T) {
	tests := map[string]string{
		"Import Tool":          "Import Tool",
		"  Import   Tool \n":   "Import Tool",
		"\tcallback\tfailure":  "callback failure",
		"":                     "",
		"   ":                  "",
		"import-tool prod 2.3": "import-tool prod 2.3",
	}
	for in, want := range tests {
		if got := NormaliseQuery(in); got != want {
			t.Errorf("NormaliseQuery(%q) = %q, want %q", in, got, want)
		}
	}
	if NameSimilarityThreshold <= 0 || NameSimilarityThreshold >= 1 {
		t.Fatalf("threshold %v out of (0,1)", NameSimilarityThreshold)
	}
}
