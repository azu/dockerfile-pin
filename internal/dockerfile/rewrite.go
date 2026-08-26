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

// addDigestAfter rewrites the first occurrence of prefix+rawRef in line, replacing any
// digest rawRef already carries. It returns line unchanged when the reference is not
// found there, which is how RewriteFile detects that it must look on a continuation line.
func addDigestAfter(line string, prefix string, rawRef string, digest string) string {
	if rawRef == "" {
		return line
	}
	idx := strings.Index(line, prefix+rawRef)
	if idx < 0 {
		return line
	}
	baseRef := rawRef
	if atIdx := strings.Index(rawRef, "@"); atIdx >= 0 {
		baseRef = rawRef[:atIdx]
	}
	end := idx + len(prefix) + len(rawRef)
	return line[:idx] + prefix + baseRef + "@" + digest + line[end:]
}

// RewriteFile applies digest pins to a Dockerfile's content.
// digests maps instruction index to digest string.
func RewriteFile(content string, instructions []FromInstruction, digests map[int]string) string {
	lines := strings.Split(content, "\n")
	for i, inst := range instructions {
		digest, ok := digests[i]
		if !ok || inst.Skip {
			continue
		}
		prefix := ""
		if inst.IsCopyFrom {
			prefix = copyFromFlag
		}
		// An instruction continued with "\" spans several lines and the reference is
		// not always written on the first one, so every line it covers is tried until
		// one of them changes.
		for lineIdx := inst.StartLine - 1; lineIdx <= lastLineIndex(inst) && lineIdx < len(lines); lineIdx++ {
			if lineIdx < 0 {
				continue
			}
			rewritten := addDigestAfter(lines[lineIdx], prefix, inst.RawRef, digest)
			if rewritten != lines[lineIdx] {
				lines[lineIdx] = rewritten
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

// lastLineIndex returns the 0-based index of the last line an instruction covers.
// EndLine is absent on instructions built by hand, so it falls back to the start line.
func lastLineIndex(inst FromInstruction) int {
	if inst.EndLine > inst.StartLine {
		return inst.EndLine - 1
	}
	return inst.StartLine - 1
}
