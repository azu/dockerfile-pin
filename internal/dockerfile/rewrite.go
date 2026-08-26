package dockerfile

import "strings"

// AddDigest inserts or replaces a digest in a FROM line.
func AddDigest(original string, rawRef string, digest string) string {
	return addDigestAfter(original, "", rawRef, digest)
}

// AddCopyFromDigest inserts or replaces a digest in the --from= flag of a COPY line.
// The flag is matched together with the reference so that the same text appearing
// elsewhere on the line — in another flag's value, or in a source path — is left alone.
func AddCopyFromDigest(original string, rawRef string, digest string) string {
	return addDigestAfter(original, copyFromFlag, rawRef, digest)
}

// addDigestAfter rewrites the first occurrence of prefix+rawRef in a single line,
// replacing any digest rawRef already carries. The line is returned unchanged when
// the reference is not found there.
func addDigestAfter(line string, prefix string, rawRef string, digest string) string {
	lines := []string{line}
	if !rewriteSpan(lines, 0, 0, prefix, rawRef, digest, defaultEscape) {
		return line
	}
	return lines[0]
}

// RewriteFile applies digest pins to a Dockerfile's content.
// digests maps instruction index to digest string.
func RewriteFile(content string, instructions []FromInstruction, digests map[int]string) string {
	result, _ := RewriteFileReport(content, instructions, digests)
	return result
}

// RewriteFileReport is RewriteFile plus the indexes of the instructions it was handed
// a digest for but could not rewrite. Reporting them keeps a caller from announcing an
// image as pinned when the file was in fact left untouched.
func RewriteFileReport(content string, instructions []FromInstruction, digests map[int]string) (string, []int) {
	lines := strings.Split(content, "\n")
	var unrewritten []int
	for i, inst := range instructions {
		digest, ok := digests[i]
		if !ok || inst.Skip {
			continue
		}
		prefix := ""
		if inst.IsCopyFrom {
			prefix = copyFromFlag
		}
		first, last := inst.StartLine-1, lastLineIndex(inst)
		if first < 0 || first >= len(lines) {
			unrewritten = append(unrewritten, i)
			continue
		}
		if last >= len(lines) {
			last = len(lines) - 1
		}
		if !rewriteSpan(lines, first, last, prefix, inst.RawRef, digest, escapeByte(inst.EscapeToken)) {
			unrewritten = append(unrewritten, i)
		}
	}
	return strings.Join(lines, "\n"), unrewritten
}

// rewriteSpan pins the reference inside the lines an instruction covers, editing lines
// in place and reporting whether it found anything to change.
//
// The search runs over the instruction as the parser sees it — one logical line — so a
// reference broken up by a "\" continuation is still found, and the digest still lands
// in the right place. Rewriting each physical line on its own would miss those and
// leave the file silently unchanged.
func rewriteSpan(lines []string, first, last int, prefix, rawRef, digest string, escape byte) bool {
	if rawRef == "" {
		return false
	}
	logical, origin := joinContinued(lines, first, last, escape)
	idx := strings.Index(logical, prefix+rawRef)
	if idx < 0 {
		return false
	}

	// Any digest the reference already carries is dropped, and the new one is written
	// where it ended.
	baseRef := rawRef
	if atIdx := strings.Index(rawRef, "@"); atIdx >= 0 {
		baseRef = rawRef[:atIdx]
	}
	cutFrom := idx + len(prefix) + len(baseRef)
	cutTo := idx + len(prefix) + len(rawRef)

	// Where the replaced range starts, and which columns of which lines it covers.
	insertLine, insertCol := last, len(lines[last])
	if cutFrom < len(origin) {
		insertLine, insertCol = origin[cutFrom].line, origin[cutFrom].col
	}
	dropped := make(map[int][2]int, 2) // line -> [from, to) columns to remove
	for k := cutFrom; k < cutTo; k++ {
		p := origin[k]
		if span, ok := dropped[p.line]; ok {
			dropped[p.line] = [2]int{span[0], p.col + 1}
		} else {
			dropped[p.line] = [2]int{p.col, p.col + 1}
		}
	}

	for i := first; i <= last; i++ {
		span, hasDrop := dropped[i]
		if !hasDrop && i != insertLine {
			continue
		}
		var sb strings.Builder
		for col := 0; col <= len(lines[i]); col++ {
			if i == insertLine && col == insertCol {
				sb.WriteString("@" + digest)
			}
			if col == len(lines[i]) {
				break
			}
			if hasDrop && col >= span[0] && col < span[1] {
				continue
			}
			sb.WriteByte(lines[i][col])
		}
		lines[i] = sb.String()
	}
	return true
}

// bytePos records where a byte of a joined instruction came from.
type bytePos struct {
	line int // index into the file's lines
	col  int // byte offset within that line
}

// defaultEscape is the character that continues a line when the Dockerfile carries no
// "# escape=" directive.
const defaultEscape = '\\'

// escapeByte returns the continuation character to strip for an instruction. A zero
// token means none was recorded, so the Dockerfile default applies. The escape
// directive accepts only "\" or "`", both ASCII, so a byte is enough.
func escapeByte(token rune) byte {
	if token == 0 {
		return defaultEscape
	}
	return byte(token)
}

// joinContinued rebuilds the text of an instruction from the lines it spans, the way
// the parser does: a continued line contributes everything before the escape character
// and the next line follows it directly. The second result maps each byte of the
// joined text back to the line and column it came from.
func joinContinued(lines []string, first, last int, escape byte) (string, []bytePos) {
	var sb strings.Builder
	origin := make([]bytePos, 0, 128)
	for i := first; i <= last; i++ {
		text := lines[i]
		if i < last {
			if idx := strings.LastIndexByte(text, escape); idx >= 0 && strings.TrimSpace(text[idx+1:]) == "" {
				text = text[:idx]
			}
		}
		for col := 0; col < len(text); col++ {
			sb.WriteByte(text[col])
			origin = append(origin, bytePos{line: i, col: col})
		}
	}
	return sb.String(), origin
}

// lastLineIndex returns the 0-based index of the last line an instruction covers.
// EndLine is absent on instructions built by hand, so it falls back to the start line.
func lastLineIndex(inst FromInstruction) int {
	if inst.EndLine > inst.StartLine {
		return inst.EndLine - 1
	}
	return inst.StartLine - 1
}
