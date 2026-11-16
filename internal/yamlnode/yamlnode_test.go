package yamlnode

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func root(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return doc.Content[0]
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantVal  string
		wantLine int
	}{
		{
			name:     "own key",
			src:      "target:\n  image: a\n",
			wantVal:  "a",
			wantLine: 2,
		},
		{
			name:     "alias value points at the anchored text",
			src:      "x: &img alpine:3\ntarget:\n  image: *img\n",
			wantVal:  "alpine:3",
			wantLine: 1,
		},
		{
			name:     "merge key",
			src:      "x: &base\n  image: a\ntarget:\n  <<: *base\n",
			wantVal:  "a",
			wantLine: 2,
		},
		{
			name:     "own key wins over a merged key written before it",
			src:      "x: &base\n  image: a\ntarget:\n  <<: *base\n  image: b\n",
			wantVal:  "b",
			wantLine: 5,
		},
		{
			name:     "earlier mapping wins in a merge sequence",
			src:      "x: &a\n  image: a\ny: &b\n  image: b\ntarget:\n  <<: [*a, *b]\n",
			wantVal:  "a",
			wantLine: 2,
		},
		{
			name:     "later mapping supplies a key the earlier one lacks",
			src:      "x: &a\n  other: a\ny: &b\n  image: b\ntarget:\n  <<: [*a, *b]\n",
			wantVal:  "b",
			wantLine: 4,
		},
		{
			name:     "merge through a merged mapping",
			src:      "x: &a\n  image: a\ny: &b\n  <<: *a\ntarget:\n  <<: *b\n",
			wantVal:  "a",
			wantLine: 2,
		},
		{
			name:     "inline mapping as merge value",
			src:      "target:\n  <<: {image: a}\n",
			wantVal:  "a",
			wantLine: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := Lookup(root(t, tt.src), "target")
			got := Lookup(target, "image")
			if got == nil {
				t.Fatal("Lookup() = nil")
			}
			if got.Value != tt.wantVal || got.Line != tt.wantLine {
				t.Errorf("Lookup() = %q at line %d, want %q at line %d", got.Value, got.Line, tt.wantVal, tt.wantLine)
			}
		})
	}
}

func TestLookup_Absent(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "missing key", src: "target:\n  other: a\n"},
		{name: "quoted << is an ordinary key", src: "x: &base\n  image: a\ntarget:\n  \"<<\": *base\n"},
		{name: "scalar is not a mapping", src: "target: text\n"},
		{name: "self-referencing anchor ends", src: "target: &t\n  <<: *t\n  other: *t\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := Lookup(root(t, tt.src), "target")
			if got := Lookup(target, "image"); got != nil {
				t.Errorf("Lookup() = %q, want nil", got.Value)
			}
		})
	}
}

func TestLookup_Nil(t *testing.T) {
	if got := Lookup(nil, "image"); got != nil {
		t.Errorf("Lookup(nil) = %v, want nil", got)
	}
	if got := Resolve(nil); got != nil {
		t.Errorf("Resolve(nil) = %v, want nil", got)
	}
}

func TestEntries(t *testing.T) {
	src := "x: &base\n  a: merged-a\n  c: merged-c\ntarget:\n  a: own-a\n  <<: *base\n  b: own-b\n"
	entries := Entries(Lookup(root(t, src), "target"))
	want := []struct{ key, value string }{
		{"a", "own-a"},
		{"b", "own-b"},
		{"c", "merged-c"},
	}
	if len(entries) != len(want) {
		t.Fatalf("Entries() returned %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i, w := range want {
		if entries[i].Key != w.key || entries[i].Value.Value != w.value {
			t.Errorf("[%d] = %s: %s, want %s: %s", i, entries[i].Key, entries[i].Value.Value, w.key, w.value)
		}
	}
}

func TestEntries_SelfReferencingAnchorEnds(t *testing.T) {
	src := "target: &t\n  a: 1\n  <<: *t\n"
	entries := Entries(Lookup(root(t, src), "target"))
	if len(entries) != 1 || entries[0].Key != "a" {
		t.Errorf("Entries() = %+v, want only a", entries)
	}
}
