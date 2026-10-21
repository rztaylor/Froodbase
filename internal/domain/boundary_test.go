package domain

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The domain package depends on nothing inside the module and on no third-party
// code: everything depends on it and it depends on nothing. Its external
// tests may also import its own test helpers under internal/domain.
func TestPackageImportsOnlyStandardLibrary(t *testing.T) {
	const ownHelpers = "github.com/rztaylor/froodbase/internal/domain"
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasSuffix(file, "_test.go") && f.Name.Name == "domain_test" && strings.HasPrefix(path, ownHelpers) {
				continue
			}
			if strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
				t.Errorf("%s imports %s; the domain package may import only the standard library", file, path)
			}
		}
	}
}
