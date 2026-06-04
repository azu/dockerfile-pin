package dockerfile

import (
	"io"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// FromInstruction represents a parsed FROM or COPY --from instruction in a Dockerfile.
type FromInstruction struct {
	ImageRef   string // image ref without digest, after ARG expansion (e.g., "node:20.11.1")
	RawRef     string // as written in Dockerfile (may contain ${VAR}, may include digest)
	Digest     string // existing digest if present (e.g., "sha256:abc...")
	Platform   string // --platform value (FROM only)
	StageName  string // AS clause name (FROM only)
	StartLine  int    // 1-based line number
	Original   string // original instruction line text
	Skip       bool
	SkipReason string
	IsCopyFrom bool // true when this instruction is COPY --from=<image>, not FROM
}

// Parse reads a Dockerfile from r and returns all FROM and COPY --from instructions.
func Parse(r io.Reader) ([]FromInstruction, error) {
	result, err := parser.Parse(r)
	if err != nil {
		return nil, err
	}

	argDefaults := map[string]string{}
	stageNames := map[string]bool{}
	for _, node := range result.AST.Children {
		switch strings.ToLower(node.Value) {
		case "arg":
			parseArgNode(node, argDefaults)
		case "from":
			if node.Next != nil {
				n := node.Next.Next
				if n != nil && strings.ToLower(n.Value) == "as" && n.Next != nil {
					stageNames[strings.ToLower(n.Next.Value)] = true
				}
			}
		}
	}

	var instructions []FromInstruction
	for _, node := range result.AST.Children {
		switch strings.ToLower(node.Value) {
		case "from":
			instructions = append(instructions, parseFromNode(node, argDefaults, stageNames))
		case "copy":
			if inst, ok := parseCopyFromNode(node, argDefaults, stageNames); ok {
				instructions = append(instructions, inst)
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

	for _, flag := range node.Flags {
		if strings.HasPrefix(flag, "--platform=") {
			inst.Platform = strings.TrimPrefix(flag, "--platform=")
		}
	}

	if node.Next == nil {
		inst.Skip = true
		inst.SkipReason = "missing image reference"
		return inst
	}

	inst.RawRef = node.Next.Value

	n := node.Next.Next
	if n != nil && strings.ToLower(n.Value) == "as" && n.Next != nil {
		inst.StageName = n.Next.Value
	}

	inst.ImageRef, inst.Digest, inst.SkipReason, inst.Skip = resolveRef(inst.RawRef, argDefaults, stageNames)
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

// parseCopyFromNode parses a COPY node and returns a FromInstruction when the
// instruction has a --from=<image> flag referencing an external image.
// Returns (inst, true) if a --from flag is present, (zero, false) otherwise.
func parseCopyFromNode(node *parser.Node, argDefaults map[string]string, stageNames map[string]bool) (FromInstruction, bool) {
	var fromValue string
	for _, flag := range node.Flags {
		if after, ok := strings.CutPrefix(flag, "--from="); ok {
			fromValue = after
			break
		}
	}
	if fromValue == "" {
		return FromInstruction{}, false
	}

	inst := FromInstruction{
		StartLine:  node.StartLine,
		Original:   node.Original,
		RawRef:     fromValue,
		IsCopyFrom: true,
	}

	// Numeric index (e.g. --from=0) refers to a build stage, not an image.
	if isNumeric(fromValue) {
		inst.Skip = true
		inst.SkipReason = "stage index reference"
		return inst, true
	}

	inst.ImageRef, inst.Digest, inst.SkipReason, inst.Skip = resolveRef(fromValue, argDefaults, stageNames)
	return inst, true
}

// resolveRef expands ARG variables in rawRef and classifies it as a pinnable
// image, a stage reference, scratch, or skipped due to unresolved variables.
// Returns (imageRef, digest, skipReason, skip).
func resolveRef(rawRef string, argDefaults map[string]string, stageNames map[string]bool) (string, string, string, bool) {
	expanded, hasUnresolved := expandVars(rawRef, argDefaults)

	if strings.ToLower(expanded) == "scratch" {
		return expanded, "", "scratch image", true
	}

	// Strip digest before comparing against stage names.
	refWithoutDigest := expanded
	if atIdx := strings.LastIndex(expanded, "@"); atIdx >= 0 {
		refWithoutDigest = expanded[:atIdx]
	}
	if stageNames[strings.ToLower(refWithoutDigest)] {
		return expanded, "", "stage reference", true
	}

	if hasUnresolved {
		return expanded, "", "unresolved ARG variable", true
	}

	if atIdx := strings.LastIndex(expanded, "@"); atIdx >= 0 {
		return expanded[:atIdx], expanded[atIdx+1:], "", false
	}
	return expanded, "", "", false
}

// isNumeric returns true if s consists entirely of ASCII digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
