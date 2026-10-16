package safefetch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Architecture rule (SSRF): outbound HTTP may only be made through this package.
// The only exception is the Docker healthcheck calling localhost in cmd/server/main.go.
func TestNoOutboundHTTPOutsideSafefetch(t *testing.T) {
	root := filepath.Join("..", "..")
	pat := regexp.MustCompile(`http\.(Get|Post|PostForm|Head|DefaultClient|NewRequest|NewRequestWithContext)\b|http\.Client\{|&http\.Client\b`)
	allow := map[string]bool{
		filepath.ToSlash(filepath.Join("cmd", "server", "main.go")): true,
	}
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "tmp" || rel == ".git" || rel == "internal/safefetch" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") || allow[rel] {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if pat.Match(b) {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("outbound HTTP outside internal/safefetch (SSRF risk): %v", offenders)
	}
}
