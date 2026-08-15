package gitlab

import (
	"strings"
	"testing"
)

func TestRewriteFile_ScalarImage(t *testing.T) {
	content := `build:
  image: node:24
  script:
    - npm ci
`
	refs, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := RewriteFile(content, refs, map[int]string{0: "sha256:aaa111"})

	if !strings.Contains(result, "image: node:24@sha256:aaa111") {
		t.Errorf("image line was not pinned:\n%s", result)
	}
	if !strings.Contains(result, "    - npm ci") {
		t.Errorf("unrelated lines were not preserved:\n%s", result)
	}
}
