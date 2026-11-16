package actions

import (
	"fmt"
	"strings"

	"github.com/azu/dockerfile-pin/internal/yamlnode"
	"gopkg.in/yaml.v3"
)

// ActionsImageRef represents a Docker image reference found in a GitHub Actions file.
type ActionsImageRef struct {
	Location   string // human-readable path, e.g. "jobs.test.container.image"
	ImageRef   string // image ref without docker:// prefix or digest
	RawRef     string // as written in the file (may include docker:// prefix and digest)
	Digest     string // existing digest if already pinned
	Line       int    // 1-based line number
	Column     int    // 1-based column of the reference, so refs sharing a line stay apart
	HasPrefix  bool   // true if the value had docker:// prefix
	Skip       bool
	SkipReason string
}

// Parse parses a GitHub Actions workflow or action file and returns Docker image references.
func Parse(content []byte) ([]ActionsImageRef, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	root := yamlnode.Resolve(doc.Content[0])
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, nil
	}

	// Detect file type by top-level keys
	if jobsNode := yamlnode.Lookup(root, "jobs"); jobsNode != nil {
		refs, err := parseWorkflow(jobsNode)
		return uniqueRefs(refs), err
	}
	if runsNode := yamlnode.Lookup(root, "runs"); runsNode != nil {
		return parseAction(runsNode)
	}
	return nil, nil
}

func parseWorkflow(jobsNode *yaml.Node) ([]ActionsImageRef, error) {
	if jobsNode.Kind != yaml.MappingNode {
		return nil, nil
	}

	var refs []ActionsImageRef
	for _, job := range yamlnode.Entries(jobsNode) {
		jobVal := job.Value
		if jobVal == nil || jobVal.Kind != yaml.MappingNode {
			continue
		}
		jobName := job.Key

		// container.image or container as string
		containerNode := yamlnode.Lookup(jobVal, "container")
		if containerNode != nil {
			refs = append(refs, parseContainer(containerNode, "jobs."+jobName+".container")...)
		}

		// services.<id>.image
		servicesNode := yamlnode.Lookup(jobVal, "services")
		if servicesNode != nil && servicesNode.Kind == yaml.MappingNode {
			for _, svc := range yamlnode.Entries(servicesNode) {
				svcVal := svc.Value
				if svcVal == nil || svcVal.Kind != yaml.MappingNode {
					continue
				}
				imageNode := yamlnode.Lookup(svcVal, "image")
				if imageNode != nil && imageNode.Kind == yaml.ScalarNode && imageNode.Value != "" {
					ref := makeRef(imageNode.Value, imageNode.Line, imageNode.Column,
						"jobs."+jobName+".services."+svc.Key+".image")
					refs = append(refs, ref)
				}
			}
		}

		// steps[*].uses with docker:// prefix
		stepsNode := yamlnode.Lookup(jobVal, "steps")
		if stepsNode != nil && stepsNode.Kind == yaml.SequenceNode {
			for _, step := range stepsNode.Content {
				step = yamlnode.Resolve(step)
				if step == nil || step.Kind != yaml.MappingNode {
					continue
				}
				usesNode := yamlnode.Lookup(step, "uses")
				if usesNode == nil || usesNode.Kind != yaml.ScalarNode {
					continue
				}
				if !strings.HasPrefix(usesNode.Value, "docker://") {
					continue
				}
				ref := makeRef(usesNode.Value, usesNode.Line, usesNode.Column,
					"jobs."+jobName+".steps.uses")
				refs = append(refs, ref)
			}
		}
	}
	return refs, nil
}

func parseContainer(node *yaml.Node, location string) []ActionsImageRef {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Value == "" {
			return nil
		}
		ref := makeRef(node.Value, node.Line, node.Column, location)
		return []ActionsImageRef{ref}
	case yaml.MappingNode:
		imageNode := yamlnode.Lookup(node, "image")
		if imageNode == nil || imageNode.Kind != yaml.ScalarNode || imageNode.Value == "" {
			return nil
		}
		ref := makeRef(imageNode.Value, imageNode.Line, imageNode.Column, location+".image")
		return []ActionsImageRef{ref}
	}
	return nil
}

func parseAction(runsNode *yaml.Node) ([]ActionsImageRef, error) {
	if runsNode.Kind != yaml.MappingNode {
		return nil, nil
	}
	imageNode := yamlnode.Lookup(runsNode, "image")
	if imageNode == nil || imageNode.Kind != yaml.ScalarNode {
		return nil, nil
	}
	value := imageNode.Value
	if !strings.HasPrefix(value, "docker://") {
		// Local Dockerfile reference (e.g., "Dockerfile" or "./Dockerfile")
		return []ActionsImageRef{{
			Location:   "runs.image",
			ImageRef:   value,
			RawRef:     value,
			Line:       imageNode.Line,
			Column:     imageNode.Column,
			Skip:       true,
			SkipReason: "local Dockerfile",
		}}, nil
	}
	ref := makeRef(value, imageNode.Line, imageNode.Column, "runs.image")
	return []ActionsImageRef{ref}, nil
}

// makeRef builds an ActionsImageRef from a raw value.
// It auto-detects the docker:// prefix and strips it from ImageRef.
// Digests are extracted from @.
func makeRef(rawValue string, line int, column int, location string) ActionsImageRef {
	hasPrefix := strings.HasPrefix(rawValue, "docker://")
	ref := ActionsImageRef{
		Location:  location,
		RawRef:    rawValue,
		Line:      line,
		Column:    column,
		HasPrefix: hasPrefix,
	}

	imageStr := rawValue
	if hasPrefix {
		imageStr = strings.TrimPrefix(rawValue, "docker://")
	}

	if atIdx := strings.Index(imageStr, "@"); atIdx >= 0 {
		ref.ImageRef = imageStr[:atIdx]
		ref.Digest = imageStr[atIdx+1:]
	} else {
		ref.ImageRef = imageStr
	}

	return ref
}

// uniqueRefs keeps one reference for each position in the file.
// Jobs that share an image through an anchor reach the same text, which must
// be reported and rewritten once.
func uniqueRefs(refs []ActionsImageRef) []ActionsImageRef {
	type position struct{ line, column int }
	seen := map[position]bool{}
	var unique []ActionsImageRef
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
