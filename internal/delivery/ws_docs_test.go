package delivery

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The WebSocket contract lives in docs/WS.md (OpenAPI cannot express it): keep it honest.
func TestWebSocketDocsDescribeEveryMessageKind(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "WS.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	for _, kind := range MessageKinds {
		if !strings.Contains(doc, "`"+kind+"`") {
			t.Errorf("docs/WS.md does not describe the %q message", kind)
		}
	}
	lower := strings.ToLower(doc)
	for _, must := range []string{"ticket", "1008", "after=", "de-duplicate"} {
		if !strings.Contains(lower, must) {
			t.Errorf("docs/WS.md is missing the %q guidance", must)
		}
	}
}

func TestEveryNotifyKindInTheCodeIsDeclared(t *testing.T) {
	declared := map[string]bool{}
	for _, k := range MessageKinds {
		declared[k] = true
	}
	re := regexp.MustCompile(`\.Notify\(\s*[^,()]+,\s*[^,()]+,\s*"([a-z_]+)"`)
	root := filepath.Join("..", "..")
	found := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == ".git" || n == "tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			found++
			if !declared[m[1]] {
				t.Errorf("%s sends WebSocket message %q that is not in delivery.MessageKinds / docs/WS.md", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found < 4 {
		t.Fatalf("expected to find the Notify calls (feed_item, alert, alert_paused, channel_disabled), found %d", found)
	}
}
