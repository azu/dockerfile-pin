package gitlab

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// GitLabImageRef represents a Docker image reference found in a GitLab CI file.
type GitLabImageRef struct {
	Location string // human-readable path, e.g. "build.image"
	ImageRef string // image ref without digest
	RawRef   string // as written in the file
	Digest   string // existing digest if already pinned
	Line     int    // 1-based line number

	Skip       bool
	SkipReason string
}

// nonJobKeywords are the global keywords of a GitLab CI file. Every other
// root-level mapping is a job, including hidden templates such as `.build`.
// `default` is deliberately absent because it carries images for all jobs.
// `image` and `services` are listed because the deprecated root form of
// `image:` may be a mapping, which would otherwise look like a job.
//
// `spec` never actually appears here: it lives in a header document that is
// separated from the configuration by `---`, and only the first document is
// read. A file using it therefore yields no references at all.
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
			return refs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parsing YAML: %w", err)
		}
		if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
			continue
		}
		refs = append(refs, parseDocument(doc.Content[0])...)
	}
}

// parseDocument reads one YAML document of a GitLab CI file. A file may hold
// more than one, because a CI component states its inputs in a header document
// that precedes the configuration.
func parseDocument(root *yaml.Node) []GitLabImageRef {
	if root.Kind != yaml.MappingNode {
		return nil
	}

	var refs []GitLabImageRef
	if ref := parseImage(findMapValue(root, "image"), "image"); ref != nil {
		refs = append(refs, *ref)
	}
	refs = append(refs, parseServices(findMapValue(root, "services"), "services")...)

	for i := 0; i+1 < len(root.Content); i += 2 {
		name := root.Content[i].Value
		value := root.Content[i+1]
		if value.Kind != yaml.MappingNode || nonJobKeywords[name] {
			continue
		}
		if ref := parseImage(findMapValue(value, "image"), name+".image"); ref != nil {
			refs = append(refs, *ref)
		}
		refs = append(refs, parseServices(findMapValue(value, "services"), name+".services")...)
	}
	return refs
}

// parseServices reads a `services:` sequence. Each entry is either a scalar or
// a mapping whose `name:` holds the reference; GitLab has no `image:` key here.
func parseServices(node *yaml.Node, location string) []GitLabImageRef {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	var refs []GitLabImageRef
	for i, entry := range node.Content {
		entryLocation := fmt.Sprintf("%s[%d]", location, i)
		switch entry.Kind {
		case yaml.ScalarNode:
			if ref := makeRef(entry, entryLocation); ref != nil {
				refs = append(refs, *ref)
			}
		case yaml.MappingNode:
			if ref := makeRef(findMapValue(entry, "name"), entryLocation+".name"); ref != nil {
				refs = append(refs, *ref)
			}
		}
	}
	return refs
}

// parseImage reads an `image:` value, which GitLab allows to be either a
// scalar or a mapping whose `name:` holds the reference.
func parseImage(node *yaml.Node, location string) *GitLabImageRef {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		return makeRef(node, location)
	case yaml.MappingNode:
		return makeRef(findMapValue(node, "name"), location+".name")
	}
	return nil
}

func makeRef(node *yaml.Node, location string) *GitLabImageRef {
	if node == nil || node.Kind != yaml.ScalarNode || node.Value == "" {
		return nil
	}
	ref := &GitLabImageRef{
		Location: location,
		ImageRef: node.Value,
		RawRef:   node.Value,
		Line:     node.Line,
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

func findMapValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
