package gitlab

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/azu/dockerfile-pin/internal/yamlnode"
	"gopkg.in/yaml.v3"
)

// GitLabImageRef represents a Docker image reference found in a GitLab CI file.
type GitLabImageRef struct {
	Location   string // human-readable path, e.g. "build.image"
	Key        string // YAML key holding the reference, empty for a sequence entry
	ImageRef   string // image ref without digest
	RawRef     string // as written in the file
	Digest     string // existing digest if already pinned
	Line       int    // 1-based line number
	Column     int    // 1-based column of the reference, so entries sharing a line stay apart
	Skip       bool
	SkipReason string
}

// nonJobKeywords are the root keys that are not jobs, job names being arbitrary.
// `default` is absent because its images are read the way a job's are.
var nonJobKeywords = map[string]bool{
	"image":     true,
	"include":   true,
	"services":  true,
	"spec":      true,
	"stages":    true,
	"variables": true,
	"workflow":  true,
}

// Parse parses a GitLab CI file and returns the Docker image references it declares.
func Parse(content []byte) ([]GitLabImageRef, error) {
	var refs []GitLabImageRef
	// A CI component declares its inputs in a header document, so a file may hold more than one.
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	for {
		var doc yaml.Node
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return uniqueRefs(atValueStart(content, refs)), nil
		}
		if err != nil {
			return nil, fmt.Errorf("parsing YAML: %w", err)
		}
		if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
			continue
		}
		refs = append(refs, parseImages(doc.Content[0])...)
	}
}

// parseImages reads one document of the file.
func parseImages(root *yaml.Node) []GitLabImageRef {
	root = yamlnode.Resolve(root)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}

	refs := parseGlobalImages(root)
	for _, entry := range yamlnode.Entries(root) {
		if entry.Value == nil || entry.Value.Kind != yaml.MappingNode || nonJobKeywords[entry.Key] {
			continue
		}
		refs = append(refs, parseJobImages(entry.Key, entry.Value)...)
	}
	return refs
}

// parseGlobalImages reads images written at the root of the file.
func parseGlobalImages(root *yaml.Node) []GitLabImageRef {
	// Declaring them at the root is deprecated, but GitLab still accepts it.
	// https://docs.gitlab.com/ci/yaml/deprecated_keywords/#globally-defined-image-services-cache-before_script-after_script
	return imageRefsIn(root, "")
}

// parseJobImages reads images written in a job.
func parseJobImages(name string, job *yaml.Node) []GitLabImageRef {
	return imageRefsIn(job, name+".")
}

func imageRefsIn(node *yaml.Node, prefix string) []GitLabImageRef {
	var refs []GitLabImageRef
	if ref := parseImageRef(yamlnode.Lookup(node, "image"), prefix+"image", "image"); ref != nil {
		refs = append(refs, *ref)
	}
	services := yamlnode.Lookup(node, "services")
	if services == nil || services.Kind != yaml.SequenceNode {
		return refs
	}
	for i, entry := range services.Content {
		location := fmt.Sprintf("%sservices[%d]", prefix, i)
		if ref := parseImageRef(yamlnode.Resolve(entry), location, ""); ref != nil {
			refs = append(refs, *ref)
		}
	}
	return refs
}

// parseImageRef reads a scalar reference or a mapping whose `name:` holds it.
func parseImageRef(node *yaml.Node, location string, scalarKey string) *GitLabImageRef {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.MappingNode {
		return makeRef(yamlnode.Lookup(node, "name"), location+".name", "name")
	}
	return makeRef(node, location, scalarKey)
}

func makeRef(node *yaml.Node, location string, key string) *GitLabImageRef {
	if node == nil || node.Kind != yaml.ScalarNode || node.Value == "" {
		return nil
	}
	ref := &GitLabImageRef{
		Location: location,
		Key:      key,
		ImageRef: node.Value,
		RawRef:   node.Value,
		Line:     node.Line,
		Column:   node.Column,
	}
	if atIdx := strings.Index(node.Value, "@"); atIdx >= 0 {
		ref.ImageRef = node.Value[:atIdx]
		ref.Digest = node.Value[atIdx+1:]
	}
	// A CI variable is substituted by the runner, so its value is unknown here.
	if strings.Contains(node.Value, "$") {
		ref.Skip = true
		ref.SkipReason = "contains CI variable"
	}
	return ref
}

// uniqueRefs keeps one reference for each position in the file.
// Jobs that share an image through an anchor reach the same text, which must
// be reported and rewritten once.
func uniqueRefs(refs []GitLabImageRef) []GitLabImageRef {
	type position struct{ line, column int }
	seen := map[position]bool{}
	var unique []GitLabImageRef
	for _, ref := range refs {
		pos := position{ref.Line, ref.Column}
		if seen[pos] {
			continue
		}
		seen[pos] = true
		unique = append(unique, ref)
	}
	return unique
}

// atValueStart moves each reference from the position of its YAML node to where
// its text begins, past any anchor or tag written before it.
func atValueStart(content []byte, refs []GitLabImageRef) []GitLabImageRef {
	lines := strings.Split(string(content), "\n")
	for i := range refs {
		refs[i].Line, refs[i].Column = yamlnode.ValueStart(lines, refs[i].Line, refs[i].Column)
	}
	return refs
}
