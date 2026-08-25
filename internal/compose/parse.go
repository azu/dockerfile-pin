package compose

import (
	"fmt"
	"strings"

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
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping at root")
	}
	servicesNode := findMapValue(root, "services")
	if servicesNode == nil {
		return nil, nil
	}
	if servicesNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("services must be a mapping")
	}
	var refs []ComposeImageRef
	for i := 0; i+1 < len(servicesNode.Content); i += 2 {
		serviceKey := servicesNode.Content[i]
		serviceVal := servicesNode.Content[i+1]
		if serviceVal.Kind != yaml.MappingNode {
			continue
		}
		serviceName := serviceKey.Value
		imageNode := findMapValue(serviceVal, "image")
		if imageNode == nil {
			continue
		}
		hasBuild := findMapValue(serviceVal, "build") != nil
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
	return refs, nil
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
