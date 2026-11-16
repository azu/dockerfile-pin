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
	if !rewriteSpan(lines, 0, 0, target{prefix: prefix, rawRef: rawRef, mountIndex: -1}, digest, defaultEscape) {
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
		first, last := inst.StartLine-1, lastLineIndex(inst)
		if first < 0 || first >= len(lines) {
			unrewritten = append(unrewritten, i)
			continue
		}
		if last >= len(lines) {
			last = len(lines) - 1
		}
		if !rewriteSpan(lines, first, last, targetOf(inst), digest, escapeByte(inst.EscapeToken)) {
			unrewritten = append(unrewritten, i)
		}
	}
	return strings.Join(lines, "\n"), unrewritten
}

// target says where in an instruction's text a reference is written.
type target struct {
	// prefix is the text that comes right before the reference. An empty prefix
	// matches the reference anywhere.
	prefix string
	rawRef string
	// mountIndex, when not negative, names the --mount flag of a RUN whose from
	// option holds the reference. prefix is not used then.
	mountIndex int
}

func targetOf(inst FromInstruction) target {
	switch {
	case inst.IsCopyFrom:
		return target{prefix: copyFromFlag, rawRef: inst.RawRef, mountIndex: -1}
	case inst.IsMountFrom:
		return target{rawRef: inst.RawRef, mountIndex: inst.MountIndex}
	}
	return target{rawRef: inst.RawRef, mountIndex: -1}
}

// find returns where the reference itself starts in the instruction text, or -1
// when it is not written there.
func (t target) find(logical string, escape byte) int {
	if t.mountIndex >= 0 {
		return findMountFrom(logical, t.mountIndex, t.rawRef, escape)
	}
	idx := strings.Index(logical, t.prefix+t.rawRef)
	if idx < 0 {
		return -1
	}
	return idx + len(t.prefix)
}

// findMountFrom returns where the value of the from option starts inside the n-th
// (0-based) --mount flag of a RUN, or -1 when that value is not rawRef.
//
// The flag and the option are located by reading the text the way BuildKit does,
// rather than by searching for "from=<ref>": the same text can appear in another
// option such as source=, and when from is repeated only the last one is used, so
// a search could pin text the build never reads and leave the real one unpinned.
func findMountFrom(logical string, n int, rawRef string, escape byte) int {
	spans := mountFlagSpans(logical, escape)
	if n >= len(spans) {
		return -1
	}
	// The options are read from the flag as the parser sees it, with quotes and
	// escapes removed, because that is the text BuildKit splits at commas. Quotes
	// may enclose the whole list, single options, or single values.
	decoded, source := unquote(logical, spans[n][0]+len(mountFlag), spans[n][1], escape)

	valueStart := -1
	offset := 0
	for _, field := range strings.Split(decoded, ",") {
		// The last from wins.
		if key, _, ok := strings.Cut(field, "="); ok && strings.ToLower(key) == mountFromKey {
			valueStart = offset + len(key) + 1
		}
		offset += len(field) + 1
	}
	if valueStart < 0 || !strings.HasPrefix(decoded[valueStart:], rawRef) || rawRef == "" {
		return -1
	}
	// The reference must also be written in one piece in the source, since the
	// digest is spliced in right after it.
	at := source[valueStart]
	if !strings.HasPrefix(logical[at:], rawRef) {
		return -1
	}
	return at
}

// unquote returns text[start:end] with quote characters and escape characters
// removed, as the Dockerfile parser reads a flag, and for each byte of the result
// the index in text it came from.
func unquote(text string, start, end int, escape byte) (string, []int) {
	var sb strings.Builder
	source := make([]int, 0, end-start)
	var quote byte
	for i := start; i < end; i++ {
		b := text[i]
		switch {
		case b == escape && quote != '\'' && i+1 < end:
			i++
			b = text[i]
		case quote == 0 && (b == '"' || b == '\''):
			quote = b
			continue
		case b == quote:
			quote = 0
			continue
		}
		sb.WriteByte(b)
		source = append(source, i)
	}
	return sb.String(), source
}

// mountFlagSpans returns the [start, end) byte ranges of the --mount flags of a RUN
// (or ONBUILD RUN) instruction, in order. Flags are the words between the
// instruction name and the first word that does not start with "--"; a word may
// hold spaces inside quotes, so "--mount=" inside a quoted value is not a flag.
func mountFlagSpans(logical string, escape byte) [][2]int {
	words := splitWords(logical, escape)
	if len(words) == 0 {
		return nil
	}
	first := 1
	if strings.EqualFold(logical[words[0][0]:words[0][1]], "onbuild") {
		first = 2
	}
	var spans [][2]int
	for _, w := range words[min(first, len(words)):] {
		text := logical[w[0]:w[1]]
		if !strings.HasPrefix(text, "--") {
			break
		}
		if strings.HasPrefix(text, mountFlag) {
			spans = append(spans, w)
		}
	}
	return spans
}

// splitWords returns the [start, end) ranges of the whitespace-separated words of
// text, keeping quoted spaces and escaped characters inside a word.
func splitWords(text string, escape byte) [][2]int {
	return splitOutsideQuotes(text, 0, len(text), escape, func(b byte) bool { return b == ' ' || b == '\t' })
}

// splitOutsideQuotes splits text[start:end] at the bytes isSep accepts, ignoring
// those inside single or double quotes or right after the escape character. Empty
// pieces are dropped.
func splitOutsideQuotes(text string, start, end int, escape byte, isSep func(byte) bool) [][2]int {
	var pieces [][2]int
	pieceStart := -1
	var quote byte
	for i := start; i < end; i++ {
		b := text[i]
		if quote == 0 && isSep(b) {
			if pieceStart >= 0 {
				pieces = append(pieces, [2]int{pieceStart, i})
				pieceStart = -1
			}
			continue
		}
		if pieceStart < 0 {
			pieceStart = i
		}
		switch {
		case b == escape && quote != '\'' && i+1 < end:
			i++
		case quote == 0 && (b == '"' || b == '\''):
			quote = b
		case b == quote:
			quote = 0
		}
	}
	if pieceStart >= 0 {
		pieces = append(pieces, [2]int{pieceStart, end})
	}
	return pieces
}

// rewriteSpan pins the reference inside the lines an instruction covers, editing lines
// in place and reporting whether it found anything to change.
//
// The search runs over the instruction as the parser sees it — one logical line — so a
// reference broken up by a "\" continuation is still found, and the digest still lands
// in the right place. Rewriting each physical line on its own would miss those and
// leave the file silently unchanged.
func rewriteSpan(lines []string, first, last int, t target, digest string, escape byte) bool {
	rawRef := t.rawRef
	if rawRef == "" {
		return false
	}
	logical, origin := joinContinued(lines, first, last, escape)
	refStart := t.find(logical, escape)
	if refStart < 0 {
		return false
	}

	// Any digest the reference already carries is dropped, and the new one is written
	// where it ended.
	baseRef := rawRef
	if atIdx := strings.Index(rawRef, "@"); atIdx >= 0 {
		baseRef = rawRef[:atIdx]
	}
	cutFrom := refStart + len(baseRef)
	cutTo := refStart + len(rawRef)

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
// https://docs.docker.com/reference/dockerfile/#escape
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
// joined text back to the line and column it came from. Comment lines between
// continued lines are left out, as the parser drops them.
func joinContinued(lines []string, first, last int, escape byte) (string, []bytePos) {
	var sb strings.Builder
	origin := make([]bytePos, 0, 128)
	for i := first; i <= last; i++ {
		text := lines[i]
		// A comment line inside a continued instruction is not part of it.
		if i > first && strings.HasPrefix(strings.TrimLeft(text, " \t"), "#") {
			continue
		}
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
