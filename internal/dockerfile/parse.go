package dockerfile

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// copyFromFlag is the COPY flag naming what to copy from: an external image, an
// earlier build stage, or a named build context. BuildKit matches flag names
// case-sensitively, so "--FROM=" is not this flag; `docker build` rejects it as an
// unknown flag rather than treating it as --from.
const copyFromFlag = "--from="

// mountFlag is the RUN flag that mounts a filesystem for one command. Its from=
// option names what to mount from: an external image, a build stage, or a named
// build context.
// https://docs.docker.com/reference/dockerfile/#run---mount
const mountFlag = "--mount="

// mountFromKey is the option of a mount that names its source. BuildKit lowercases
// option keys before comparing them, so "FROM=" is this option too.
const mountFromKey = "from"

// Reasons reported in FromInstruction.SkipReason when an instruction is not pinnable.
const (
	SkipMissingRef    = "missing image reference"
	SkipScratch       = "scratch image"
	SkipStageRef      = "stage reference"
	SkipStageIndex    = "stage index reference"
	SkipUnresolvedARG = "unresolved ARG variable"
	SkipCopyFromVar   = "unexpanded variable in COPY --from"
	SkipMountFromVar  = "unexpanded variable in RUN --mount from"
	SkipMountSyntax   = "unsupported RUN --mount syntax"
)

// FromInstruction represents a pinnable image reference in a Dockerfile: a FROM
// instruction, the --from= flag of a COPY instruction, or the from= option of a
// RUN --mount flag.
type FromInstruction struct {
	ImageRef   string // image ref without digest, after ARG expansion (e.g., "node:20.11.1")
	RawRef     string // as written in Dockerfile (may contain ${VAR}, may include digest)
	Digest     string // existing digest if present (e.g., "sha256:abc...")
	Platform   string // --platform value (FROM only)
	StageName  string // AS clause name (FROM only)
	StartLine  int    // 1-based line where the instruction starts
	EndLine    int    // 1-based line where the instruction ends (larger than StartLine when continued with "\")
	Original   string // original instruction text, with any line continuations joined
	Skip       bool
	SkipReason string
	IsCopyFrom bool // true for COPY --from=<ref>
	// IsMountFrom is true for RUN --mount=...,from=<ref>. MountIndex is then the
	// position of that --mount flag among the instruction's --mount flags, which
	// tells the rewriter which flag holds the reference.
	IsMountFrom bool
	MountIndex  int
	// EscapeToken is the character that continues a line, from the "# escape="
	// parser directive. Zero means the Dockerfile default, a backslash.
	// https://docs.docker.com/reference/dockerfile/#escape
	EscapeToken rune
}

// Parse reads a Dockerfile from r and returns every pinnable image reference:
// each FROM instruction, each COPY --from=<ref>, and each RUN --mount=...,from=<ref>.
func Parse(r io.Reader) ([]FromInstruction, error) {
	result, err := parser.Parse(r)
	if err != nil {
		return nil, err
	}

	// Every stage name is collected up front for COPY --from, so that a name declared
	// further down the file is still recognised as a stage rather than mistaken for an
	// image to pin. Using one that way is a build error ("cannot copy from stage %q, it
	// needs to be defined before current stage %q"), but it is an error about stage
	// order, not an unpinned image, so there is nothing here to rewrite.
	// FROM does not get the same treatment: BuildKit resolves each FROM against the
	// stages declared above it only, so a name defined later really is an image there,
	// and seenStages below grows as the file is walked.
	allStages := collectStageNames(result.AST)

	argDefaults := map[string]string{}
	seenStages := map[string]bool{}
	var instructions []FromInstruction

	for _, node := range result.AST.Children {
		switch strings.ToLower(node.Value) {
		case "arg":
			// Collected in file order: an ARG below a FROM is scoped to the stage it
			// opens, so it must not expand a variable in that FROM.
			parseArgNode(node, argDefaults)
		case "from":
			inst := parseFromNode(node, argDefaults, seenStages)
			instructions = append(instructions, inst)
			if inst.StageName != "" {
				seenStages[strings.ToLower(inst.StageName)] = true
			}
		case "copy":
			if inst, ok := parseCopyFromNode(node, allStages); ok {
				instructions = append(instructions, inst)
			}
		case "run":
			instructions = append(instructions, parseRunMountNode(node, allStages)...)
		case "onbuild":
			// The wrapped instruction is only recorded into this image's config here;
			// it runs later, inside whichever build uses this image as its base, and is
			// resolved against *that* Dockerfile's stages. The names declared in this
			// file are gone by then, so none of them is passed: a bare name in an
			// ONBUILD trigger is an image to resolve, not a stage to skip.
			child := onbuildChild(node)
			if child == nil {
				continue
			}
			switch strings.ToLower(child.Value) {
			case "copy":
				if inst, ok := parseCopyFromNode(child, nil); ok {
					instructions = append(instructions, inst)
				}
			case "run":
				instructions = append(instructions, parseRunMountNode(child, nil)...)
			}
		}
	}

	for i := range instructions {
		instructions[i].EscapeToken = result.EscapeToken
	}

	return instructions, nil
}

// onbuildChild returns the instruction an ONBUILD wraps, or nil if it wraps nothing.
// The parser hangs that instruction off an unnamed node and leaves its position unset,
// so the ONBUILD node supplies the line span and source text the rewriter needs.
func onbuildChild(node *parser.Node) *parser.Node {
	if node.Next == nil || len(node.Next.Children) != 1 {
		return nil
	}
	child := *node.Next.Children[0]
	child.StartLine = node.StartLine
	child.EndLine = node.EndLine
	child.Original = node.Original
	return &child
}

// collectStageNames returns every name introduced by a "FROM ... AS <name>" clause,
// lowercased because BuildKit looks stage names up case-insensitively.
func collectStageNames(root *parser.Node) map[string]bool {
	names := map[string]bool{}
	for _, node := range root.Children {
		if strings.ToLower(node.Value) != "from" || node.Next == nil {
			continue
		}
		if n := node.Next.Next; n != nil && strings.ToLower(n.Value) == "as" && n.Next != nil {
			names[strings.ToLower(n.Next.Value)] = true
		}
	}
	return names
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
		EndLine:   node.EndLine,
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
		inst.SkipReason = SkipMissingRef
		return inst
	}

	inst.RawRef = node.Next.Value

	// Check for AS clause (Next.Next = "as", Next.Next.Next = stage name)
	n := node.Next.Next
	if n != nil && strings.ToLower(n.Value) == "as" && n.Next != nil {
		inst.StageName = n.Next.Value
	}

	inst.ImageRef, inst.Digest, inst.SkipReason, inst.Skip = resolveRef(inst.RawRef, argDefaults, stageNames)
	return inst
}

// parseCopyFromNode parses a COPY node, returning a FromInstruction when the
// instruction carries a --from= flag. The second result is false for a plain COPY,
// which copies from the build context and has nothing to pin.
func parseCopyFromNode(node *parser.Node, stageNames map[string]bool) (FromInstruction, bool) {
	fromValue, ok := copyFromValue(node.Flags)
	if !ok {
		return FromInstruction{}, false
	}

	inst := FromInstruction{
		StartLine:  node.StartLine,
		EndLine:    node.EndLine,
		Original:   node.Original,
		RawRef:     fromValue,
		ImageRef:   fromValue,
		IsCopyFrom: true,
	}

	if fromValue == "" {
		inst.Skip = true
		inst.SkipReason = SkipMissingRef
		return inst, true
	}

	// BuildKit reads the value as a stage index before anything else, so a numeric
	// --from always selects a build stage by position and never names an image.
	if isStageIndex(fromValue) {
		inst.Skip = true
		inst.SkipReason = SkipStageIndex
		return inst, true
	}

	// Unlike FROM, whose base name is expanded before it is resolved, the --from value
	// is read verbatim: CopyCommand.Expand covers --chown, --chmod and the paths, but
	// not --from. A variable written here makes the build fail to parse the stage name
	// (moby/buildkit#2374), so the value is left alone rather than pinned to whatever
	// the variable would have expanded to.
	if strings.ContainsRune(fromValue, '$') {
		inst.Skip = true
		inst.SkipReason = SkipCopyFromVar
		return inst, true
	}

	inst.ImageRef, inst.Digest, inst.SkipReason, inst.Skip = classifyRef(fromValue, stageNames)
	return inst, true
}

// parseRunMountNode returns one FromInstruction for each --mount flag of a RUN node
// that names a source with from=. A mount without from= reads the build context or
// nothing at all, so it has nothing to pin.
//
// Reading and writing are kept apart: this function decides whether a reference is
// written in the one form the rewriter handles, and marks every other form as
// skipped. The rewriter then never has to guess where a reference is.
func parseRunMountNode(node *parser.Node, stageNames map[string]bool) []FromInstruction {
	var instructions []FromInstruction
	written := plainMountFlags(node.Original)
	mountIndex := -1
	for _, flag := range node.Flags {
		value, ok := strings.CutPrefix(flag, mountFlag)
		if !ok {
			continue
		}
		mountIndex++
		fromValue, ok := mountFromOption(value)
		// BuildKit treats an empty from= like a missing one.
		if !ok || fromValue == "" {
			continue
		}

		inst := FromInstruction{
			StartLine:   node.StartLine,
			EndLine:     node.EndLine,
			Original:    node.Original,
			RawRef:      fromValue,
			ImageRef:    fromValue,
			IsMountFrom: true,
			MountIndex:  mountIndex,
		}

		// BuildKit refuses a variable here ("'from' doesn't support variable
		// expansion, define alias stage instead"), so the value is left alone rather
		// than pinned to whatever the variable would have expanded to.
		if strings.ContainsRune(fromValue, '$') {
			inst.Skip = true
			inst.SkipReason = SkipMountFromVar
			instructions = append(instructions, inst)
			continue
		}

		// Unlike COPY --from, a number is not a stage index here: BuildKit looks the
		// value up among stage names only, and otherwise pulls it as an image.
		inst.ImageRef, inst.Digest, inst.SkipReason, inst.Skip = classifyRef(fromValue, stageNames)

		supported := mountIndex < len(written) && isSupportedMount(written[mountIndex], value, fromValue)
		if !inst.Skip && !supported {
			inst.Skip = true
			inst.SkipReason = SkipMountSyntax
		}
		instructions = append(instructions, inst)
	}
	return instructions
}

// mountFromOption returns the from option of a --mount flag value, the way BuildKit
// reads it: the value is one CSV record of key=value options, the key is compared
// without regard to case, and when from is repeated the last one wins.
func mountFromOption(mount string) (value string, found bool) {
	reader := csv.NewReader(strings.NewReader(mount))
	reader.LazyQuotes = true
	fields, err := reader.Read()
	if err != nil {
		return "", false
	}
	for _, field := range fields {
		k, v, ok := strings.Cut(field, "=")
		if ok && strings.ToLower(k) == mountFromKey {
			value, found = v, true
		}
	}
	return value, found
}

// isSupportedMount reports whether a --mount flag is written in one of the
// supported forms. writtenFlag is the flag as it stands in the source, parsed the
// value the Dockerfile parser returned for it, and fromValue the reference.
//
// The option list read from the source has to equal the one the parser returned.
// They differ when the flag uses quotes or escape characters in a way no supported
// form covers, since the parser removes those.
func isSupportedMount(writtenFlag, parsed, fromValue string) bool {
	writtenValue, ok := strings.CutPrefix(writtenFlag, mountFlag)
	if !ok {
		return false
	}
	options, _, ok := locateMountFrom(writtenValue, fromValue)
	return ok && options == parsed
}

// locateMountFrom reads a --mount value as it is written and returns the option
// list without quotes and where the reference starts in written. Two syntaxes are
// supported, each read on its own:
//
//	type=bind,from=image:tag,target=/mnt      an option list
//	type=bind,from="image:tag",target=/mnt    its variant with the from value in quotes
//	"type=bind,from=image:tag,target=/mnt"    a quoted option list
//
// Quotes are " or '. In both syntaxes the options are separated by commas and there
// is exactly one from option, whose value is want. Anything else is not supported.
func locateMountFrom(written, want string) (options string, offset int, ok bool) {
	if written == "" {
		return "", 0, false
	}
	if q := written[0]; q == '"' || q == '\'' {
		return locateInQuotedOptionList(written, q, want)
	}
	return locateInOptionList(written, want)
}

// locateInOptionList reads an option list written without quotes, or with quotes
// around the from value and nowhere else.
func locateInOptionList(written, want string) (options string, offset int, ok bool) {
	switch strings.Count(written, `"`) + strings.Count(written, "'") {
	case 0:
		offset, ok = plainFromOffset(written, want)
		return written, offset, ok
	case 2:
		for _, q := range []string{`"`, "'"} {
			if offset, ok = plainFromOffset(written, q+want+q); ok {
				return strings.ReplaceAll(written, q, ""), offset + 1, true
			}
		}
	}
	return "", 0, false
}

// locateInQuotedOptionList reads an option list enclosed in one pair of quotes, with
// no other quote inside.
func locateInQuotedOptionList(written string, quote byte, want string) (options string, offset int, ok bool) {
	if len(written) < 2 || written[len(written)-1] != quote {
		return "", 0, false
	}
	inner := written[1 : len(written)-1]
	if strings.ContainsAny(inner, `"'`) {
		return "", 0, false
	}
	offset, ok = plainFromOffset(inner, want)
	return inner, offset + 1, ok
}

// plainFromOffset returns where the value of the only from option starts in an
// option list split at every comma. It fails when there is no from option, more
// than one, or its value is not want.
func plainFromOffset(mount, want string) (int, bool) {
	found, at := 0, 0
	offset := 0
	for _, field := range strings.Split(mount, ",") {
		if key, value, ok := strings.Cut(field, "="); ok && strings.ToLower(key) == mountFromKey {
			found++
			if value == want {
				at = offset + len(key) + 1
			} else {
				return 0, false
			}
		}
		offset += len(field) + 1
	}
	return at, found == 1
}

// plainMountFlags returns the --mount flags of a RUN (or ONBUILD RUN) instruction as
// they are written, reading the text as words separated by spaces or tabs with no
// regard to quotes. Flags are the words between the instruction name and the first
// word that does not start with "--".
//
// A flag written with quotes or escapes comes out different from the one the
// Dockerfile parser returns, which is how such a flag is recognised as unsupported.
func plainMountFlags(instruction string) []string {
	var flags []string
	for _, span := range plainMountFlagSpans(instruction) {
		flags = append(flags, instruction[span[0]:span[1]])
	}
	return flags
}

// plainMountFlagSpans returns the [start, end) byte ranges of those flags.
func plainMountFlagSpans(instruction string) [][2]int {
	var spans [][2]int
	word := 0 // index of the current word
	skip := 1 // words that name the instruction
	for i := 0; i < len(instruction); {
		for i < len(instruction) && (instruction[i] == ' ' || instruction[i] == '\t') {
			i++
		}
		start := i
		for i < len(instruction) && instruction[i] != ' ' && instruction[i] != '\t' {
			i++
		}
		if start == i {
			break
		}
		text := instruction[start:i]
		if word == 0 && strings.EqualFold(text, "onbuild") {
			skip = 2
		}
		if word >= skip {
			if !strings.HasPrefix(text, "--") {
				break
			}
			if strings.HasPrefix(text, mountFlag) {
				spans = append(spans, [2]int{start, i})
			}
		}
		word++
	}
	return spans
}

// copyFromValue returns the value of the --from= flag among a COPY node's flags.
func copyFromValue(flags []string) (string, bool) {
	for _, flag := range flags {
		if value, ok := strings.CutPrefix(flag, copyFromFlag); ok {
			return value, true
		}
	}
	return "", false
}

// isStageIndex reports whether a --from value selects a build stage by position.
// It mirrors BuildKit, which classifies the value with strconv.Atoi.
func isStageIndex(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// resolveRef expands ARG variables in rawRef, as BuildKit does for a FROM base name,
// and classifies the result. It returns the image ref, any digest already written on
// it, and why it should be skipped (if it should).
func resolveRef(rawRef string, argDefaults map[string]string, stageNames map[string]bool) (imageRef, digest, skipReason string, skip bool) {
	expanded, hasUnresolved := expandVars(rawRef, argDefaults)
	if hasUnresolved {
		// Still classified first: a name that is scratch or a stage is recognisable
		// even when some other part of the ref did not expand.
		if ref, dgst, reason, skip := classifyRef(expanded, stageNames); skip {
			return ref, dgst, reason, skip
		}
		return expanded, "", SkipUnresolvedARG, true
	}
	return classifyRef(expanded, stageNames)
}

// classifyRef sorts an image ref that needs no further expansion into a pinnable
// image, a stage reference, or scratch, splitting off any digest it already carries.
func classifyRef(ref string, stageNames map[string]bool) (imageRef, digest, skipReason string, skip bool) {
	if strings.EqualFold(ref, "scratch") {
		return ref, "", SkipScratch, true
	}

	// BuildKit matches the whole value against its stage names, digest included, so
	// "builder@sha256:..." finds no stage named "builder" and is an image reference.
	if stageNames[strings.ToLower(ref)] {
		return ref, "", SkipStageRef, true
	}

	if atIdx := strings.LastIndex(ref, "@"); atIdx >= 0 {
		return ref[:atIdx], ref[atIdx+1:], "", false
	}
	return ref, "", "", false
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
