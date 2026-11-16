package compose

import (
	"fmt"
	"strings"

	"github.com/azu/dockerfile-pin/internal/yamlnode"
	"gopkg.in/yaml.v3"
)

type ComposeImageRef struct {
	ServiceName string
	ImageRef    string // without digest
	RawRef      string // as written
	Digest      string
	Line        int // 1-based line number of image: value
	Column      int // 1-based column of the value, so images sharing a line stay apart
	Skip        bool
	SkipReason  string
}

func Parse(content []byte) ([]ComposeImageRef, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("invalid YAML document")
	}
	root := yamlnode.Resolve(doc.Content[0])
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping at root")
	}
	servicesNode := yamlnode.Lookup(root, "services")
	if servicesNode == nil {
		return nil, nil
	}
	if servicesNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("services must be a mapping")
	}
	var refs []ComposeImageRef
	for _, service := range yamlnode.Entries(servicesNode) {
		serviceVal := service.Value
		if serviceVal == nil || serviceVal.Kind != yaml.MappingNode {
			continue
		}
		serviceName := service.Key
		imageNode := yamlnode.Lookup(serviceVal, "image")
		if imageNode == nil || imageNode.Kind != yaml.ScalarNode {
			continue
		}
		hasBuild := yamlnode.Lookup(serviceVal, "build") != nil
		rawRef := imageNode.Value
		ref := ComposeImageRef{
			ServiceName: serviceName,
			RawRef:      rawRef,
			Line:        imageNode.Line,
			Column:      imageNode.Column,
		}
		if hasBuild {
			ref.ImageRef = rawRef
			ref.Skip = true
			ref.SkipReason = "has build directive"
			refs = append(refs, ref)
			continue
		}
		if atIdx := strings.Index(rawRef, "@"); atIdx >= 0 {
			ref.ImageRef = rawRef[:atIdx]
			ref.Digest = rawRef[atIdx+1:]
		} else {
			ref.ImageRef = rawRef
		}
		refs = append(refs, ref)
	}
	return uniqueRefs(refs), nil
}

// uniqueRefs keeps one reference for each position in the file.
// Services that share an image through an anchor reach the same text, which
// must be reported and rewritten once. It is skipped only when every service
// reaching it skips it.
func uniqueRefs(refs []ComposeImageRef) []ComposeImageRef {
	type position struct{ line, column int }
	index := map[position]int{}
	var unique []ComposeImageRef
	for _, ref := range refs {
		pos := position{ref.Line, ref.Column}
		i, found := index[pos]
		if !found {
			index[pos] = len(unique)
			unique = append(unique, ref)
			continue
		}
		if unique[i].Skip && !ref.Skip {
			unique[i] = ref
		}
	}
	return unique
}
