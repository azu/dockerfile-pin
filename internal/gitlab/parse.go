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
	Location   string // human-readable path, e.g. "build.image"
	Key        string // YAML key holding the reference, empty for a sequence entry
	ImageRef   string // image ref without digest
	RawRef     string // as written in the file
	Digest     string // existing digest if already pinned
	Line       int    // 1-based line number
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
			return refs, nil
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
	if root.Kind != yaml.MappingNode {
		return nil
	}

	refs := parseGlobalImages(root)
	for i := 0; i+1 < len(root.Content); i += 2 {
		name := root.Content[i].Value
		value := root.Content[i+1]
		if value.Kind != yaml.MappingNode || nonJobKeywords[name] {
			continue
		}
		refs = append(refs, parseJobImages(name, value)...)
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
	if ref := parseImageRef(findMapValue(node, "image"), prefix+"image", "image"); ref != nil {
		refs = append(refs, *ref)
	}
	services := findMapValue(node, "services")
	if services == nil || services.Kind != yaml.SequenceNode {
		return refs
	}
	for i, entry := range services.Content {
		location := fmt.Sprintf("%sservices[%d]", prefix, i)
		if ref := parseImageRef(entry, location, ""); ref != nil {
			refs = append(refs, *ref)
		}
	}
	return refs
}

// parseImageRef reads a scalar reference or a mapping whose `name:` holds it.
// A service entry is keyed on `name:`, not on `image:` as Docker Compose is.
func parseImageRef(node *yaml.Node, location string, scalarKey string) *GitLabImageRef {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.MappingNode {
		return makeRef(findMapValue(node, "name"), location+".name", "name")
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
