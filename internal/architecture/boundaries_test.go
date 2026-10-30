package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Pure, reusable models must not acquire an HTTP/DB dependency when the second
// service arrives. Feature packages must never import the cosmic composition root.
func TestPackageBoundaries(t *testing.T) {
	const prefix = "github.com/yomezza/cosmosimulator/internal/"
	root := ".."
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		owner := filepath.ToSlash(rel)
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		pure := owner == "universe" || owner == "geography" || owner == "resources" || owner == "planet/terrain" || owner == "planet/territory" || owner == "planet/scene" || owner == "planet/pathing"
		for _, spec := range f.Imports {
			target, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if owner != "server" && (target == prefix+"server" || strings.HasPrefix(target, prefix+"server/")) {
				t.Errorf("%s imports composition root %s", path, target)
			}
			if pure {
				if strings.HasPrefix(target, "github.com/jackc/") || target == "net/http" {
					t.Errorf("pure package %s imports I/O %s", owner, target)
				}
				if strings.HasPrefix(target, prefix) {
					dep := strings.TrimPrefix(target, prefix)
					allowed := owner == "universe" && dep == "resources" || owner == "geography" && dep == "universe" || owner == "planet/terrain" && dep == "geography" || owner == "planet/scene" && (dep == "geography" || dep == "planet/terrain")
					if !allowed {
						t.Errorf("pure package %s imports feature %s", owner, dep)
					}
				}
			}
			if strings.HasPrefix(owner, "platform/") && strings.HasPrefix(target, prefix) && !strings.HasPrefix(target, prefix+"platform/") {
				t.Errorf("platform %s imports game feature %s", owner, target)
			}
		}
		// Parsing the import graph is intentional; this does not assert file counts or function names.
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
