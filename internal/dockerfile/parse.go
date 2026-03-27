package dockerfile

import (
	"io"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// FromInstruction represents a parsed FROM instruction in a Dockerfile.
type FromInstruction struct {
	ImageRef   string // image ref without digest, after ARG expansion (e.g., "node:20.11.1")
	RawRef     string // as written in Dockerfile (may contain ${VAR}, may include digest)
	Digest     string // existing digest if present (e.g., "sha256:abc...")
	Platform   string // --platform value
	StageName  string // AS clause name
	StartLine  int    // 1-based line number
	Original   string // original FROM line text
	Skip       bool
	SkipReason string
}

// Parse reads a Dockerfile from r and returns all FROM instructions.
func Parse(r io.Reader) ([]FromInstruction, error) {
	result, err := parser.Parse(r)
	if err != nil {
		return nil, err
	}

	argDefaults := map[string]string{}
	stageNames := map[string]bool{}
	var instructions []FromInstruction

	for _, node := range result.AST.Children {
		switch strings.ToLower(node.Value) {
		case "arg":
			parseArgNode(node, argDefaults)
		case "from":
			inst := parseFromNode(node, argDefaults, stageNames)
			instructions = append(instructions, inst)
			if inst.StageName != "" {
				stageNames[strings.ToLower(inst.StageName)] = true
			}
		}
	}

	return instructions, nil
}

// parseArgNode extracts ARG defaults from an ARG node and stores them in defaults map.
func parseArgNode(node *parser.Node, defaults map[string]string) {
	if node.Next == nil {
		return
	}
	val := node.Next.Value
	if idx := strings.IndexByte(val, '='); idx >= 0 {
		key := val[:idx]
		value := val[idx+1:]
		defaults[key] = value
	}
	// ARG without default: do not add to map (will remain unresolved)
}

// parseFromNode parses a FROM AST node into a FromInstruction.
func parseFromNode(node *parser.Node, argDefaults map[string]string, stageNames map[string]bool) FromInstruction {
	inst := FromInstruction{
		StartLine: node.StartLine,
		Original:  node.Original,
	}

	// Extract --platform flag
	for _, flag := range node.Flags {
		if strings.HasPrefix(flag, "--platform=") {
			inst.Platform = strings.TrimPrefix(flag, "--platform=")
		}
	}

	// The image ref is the first token after FROM (node.Next)
	if node.Next == nil {
		inst.Skip = true
		inst.SkipReason = "missing image reference"
		return inst
	}

	rawRef := node.Next.Value
	inst.RawRef = rawRef

	// Check for AS clause (Next.Next = "as", Next.Next.Next = stage name)
	n := node.Next.Next
	if n != nil && strings.ToLower(n.Value) == "as" && n.Next != nil {
		inst.StageName = n.Next.Value
	}

	// Handle scratch image
	if strings.ToLower(rawRef) == "scratch" {
		inst.Skip = true
		inst.SkipReason = "scratch image"
		inst.ImageRef = rawRef
		return inst
	}

	// Expand ARG variables in the ref
	expanded, hasUnresolved := expandVars(rawRef, argDefaults)

	// Check if the expanded ref is a stage reference
	// A stage reference is when the ref (without tag/digest) matches a known stage name
	refWithoutDigest := expanded
	if atIdx := strings.LastIndex(expanded, "@"); atIdx >= 0 {
		refWithoutDigest = expanded[:atIdx]
	}
	// Stage names don't contain "/" or ":" or "."
	refLower := strings.ToLower(refWithoutDigest)
	if stageNames[refLower] {
		inst.Skip = true
		inst.SkipReason = "stage reference"
		inst.ImageRef = expanded
		return inst
	}

	// If expansion produced unresolved variables, skip
	if hasUnresolved {
		inst.Skip = true
		inst.SkipReason = "unresolved ARG variable"
		inst.ImageRef = expanded
		return inst
	}

	// Split digest at "@" if present
	if atIdx := strings.LastIndex(expanded, "@"); atIdx >= 0 {
		inst.ImageRef = expanded[:atIdx]
		inst.Digest = expanded[atIdx+1:]
	} else {
		inst.ImageRef = expanded
	}

	return inst
}

// expandVars expands ${VAR} and $VAR syntax using the provided defaults map.
// Returns the expanded string and whether any variables were unresolved.
func expandVars(s string, defaults map[string]string) (string, bool) {
	var sb strings.Builder
	hasUnresolved := false
	i := 0
	for i < len(s) {
		if s[i] != '$' {
			sb.WriteByte(s[i])
			i++
			continue
		}
		// Found '$'
		i++ // skip '$'
		if i >= len(s) {
			sb.WriteByte('$')
			break
		}

		if s[i] == '{' {
			// ${VAR} syntax
			i++ // skip '{'
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				// malformed, keep as-is
				sb.WriteString("${")
				sb.WriteString(s[i:])
				hasUnresolved = true
				break
			}
			varName := s[i : i+end]
			i += end + 1 // skip past '}'
			if val, ok := defaults[varName]; ok {
				sb.WriteString(val)
			} else {
				// unresolved: keep original
				sb.WriteString("${")
				sb.WriteString(varName)
				sb.WriteByte('}')
				hasUnresolved = true
			}
		} else if isAlphaNumUnderscore(s[i]) {
			// $VAR syntax
			end := i
			for end < len(s) && isAlphaNumUnderscore(s[end]) {
				end++
			}
			varName := s[i:end]
			i = end
			if val, ok := defaults[varName]; ok {
				sb.WriteString(val)
			} else {
				// unresolved: keep original
				sb.WriteByte('$')
				sb.WriteString(varName)
				hasUnresolved = true
			}
		} else {
			// not a variable reference, keep '$'
			sb.WriteByte('$')
		}
	}
	return sb.String(), hasUnresolved
}

// isAlphaNumUnderscore returns true if b is a valid variable name character.
func isAlphaNumUnderscore(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}
