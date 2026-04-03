package actions

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ActionsImageRef represents a Docker image reference found in a GitHub Actions file.
type ActionsImageRef struct {
	Location   string // human-readable path, e.g. "jobs.test.container.image"
	ImageRef   string // image ref without docker:// prefix or digest
	RawRef     string // as written in the file (may include docker:// prefix and digest)
	Digest     string // existing digest if already pinned
	Line       int    // 1-based line number
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
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil
	}

	// Detect file type by top-level keys
	if jobsNode := findMapValue(root, "jobs"); jobsNode != nil {
		return parseWorkflow(jobsNode)
	}
	if runsNode := findMapValue(root, "runs"); runsNode != nil {
		return parseAction(runsNode)
	}
	return nil, nil
}

func parseWorkflow(jobsNode *yaml.Node) ([]ActionsImageRef, error) {
	if jobsNode.Kind != yaml.MappingNode {
		return nil, nil
	}

	var refs []ActionsImageRef
	for i := 0; i+1 < len(jobsNode.Content); i += 2 {
		jobKey := jobsNode.Content[i]
		jobVal := jobsNode.Content[i+1]
		if jobVal.Kind != yaml.MappingNode {
			continue
		}
		jobName := jobKey.Value

		// container.image or container as string
		containerNode := findMapValue(jobVal, "container")
		if containerNode != nil {
			refs = append(refs, parseContainer(containerNode, "jobs."+jobName+".container")...)
		}

		// services.<id>.image
		servicesNode := findMapValue(jobVal, "services")
		if servicesNode != nil && servicesNode.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(servicesNode.Content); j += 2 {
				svcKey := servicesNode.Content[j]
				svcVal := servicesNode.Content[j+1]
				if svcVal.Kind != yaml.MappingNode {
					continue
				}
				imageNode := findMapValue(svcVal, "image")
				if imageNode != nil && imageNode.Kind == yaml.ScalarNode && imageNode.Value != "" {
					ref := makeRef(imageNode.Value, imageNode.Line, false,
						"jobs."+jobName+".services."+svcKey.Value+".image")
					refs = append(refs, ref)
				}
			}
		}

		// steps[*].uses with docker:// prefix
		stepsNode := findMapValue(jobVal, "steps")
		if stepsNode != nil && stepsNode.Kind == yaml.SequenceNode {
			for _, step := range stepsNode.Content {
				if step.Kind != yaml.MappingNode {
					continue
				}
				usesNode := findMapValue(step, "uses")
				if usesNode == nil || usesNode.Kind != yaml.ScalarNode {
					continue
				}
				if !strings.HasPrefix(usesNode.Value, "docker://") {
					continue
				}
				ref := makeRef(usesNode.Value, usesNode.Line, true,
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
		hasPrefix := strings.HasPrefix(node.Value, "docker://")
		ref := makeRef(node.Value, node.Line, hasPrefix, location)
		return []ActionsImageRef{ref}
	case yaml.MappingNode:
		imageNode := findMapValue(node, "image")
		if imageNode == nil || imageNode.Kind != yaml.ScalarNode || imageNode.Value == "" {
			return nil
		}
		hasPrefix := strings.HasPrefix(imageNode.Value, "docker://")
		ref := makeRef(imageNode.Value, imageNode.Line, hasPrefix, location+".image")
		return []ActionsImageRef{ref}
	}
	return nil
}

func parseAction(runsNode *yaml.Node) ([]ActionsImageRef, error) {
	if runsNode.Kind != yaml.MappingNode {
		return nil, nil
	}
	imageNode := findMapValue(runsNode, "image")
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
			Skip:       true,
			SkipReason: "local Dockerfile",
		}}, nil
	}
	ref := makeRef(value, imageNode.Line, true, "runs.image")
	return []ActionsImageRef{ref}, nil
}

// makeRef builds an ActionsImageRef from a raw value. If hasDockerPrefix is true,
// the docker:// prefix is stripped from ImageRef. Digests are extracted from @.
func makeRef(rawValue string, line int, hasDockerPrefix bool, location string) ActionsImageRef {
	ref := ActionsImageRef{
		Location:  location,
		RawRef:    rawValue,
		Line:      line,
		HasPrefix: hasDockerPrefix,
	}

	imageStr := rawValue
	if hasDockerPrefix {
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
