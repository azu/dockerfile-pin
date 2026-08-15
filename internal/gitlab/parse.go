package gitlab

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// GitLabImageRef represents a Docker image reference found in a GitLab CI file.
type GitLabImageRef struct {
	Location string // human-readable path, e.g. "build.image"
	ImageRef string // image ref without digest
	RawRef   string // as written in the file
	Line     int    // 1-based line number
}

// Parse parses a GitLab CI file and returns the Docker image references it declares.
func Parse(content []byte) ([]GitLabImageRef, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil
	}

	var refs []GitLabImageRef
	for i := 0; i+1 < len(root.Content); i += 2 {
		name := root.Content[i].Value
		value := root.Content[i+1]
		if value.Kind != yaml.MappingNode {
			continue
		}
		if ref := parseImage(findMapValue(value, "image"), name+".image"); ref != nil {
			refs = append(refs, *ref)
		}
		refs = append(refs, parseServices(findMapValue(value, "services"), name+".services")...)
	}
	return refs, nil
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
	return &GitLabImageRef{
		Location: location,
		ImageRef: node.Value,
		RawRef:   node.Value,
		Line:     node.Line,
	}
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
