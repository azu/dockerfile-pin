package dockerfile

import "strings"

// AddDigest inserts or replaces a digest in a FROM line.
func AddDigest(original string, rawRef string, digest string) string {
	if atIdx := strings.Index(rawRef, "@"); atIdx >= 0 {
		baseRef := rawRef[:atIdx]
		newRef := baseRef + "@" + digest
		return strings.Replace(original, rawRef, newRef, 1)
	}
	newRef := rawRef + "@" + digest
	return strings.Replace(original, rawRef, newRef, 1)
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
		lineIdx := inst.StartLine - 1
		if lineIdx >= 0 && lineIdx < len(lines) {
			lines[lineIdx] = AddDigest(lines[lineIdx], inst.RawRef, digest)
		}
	}
	return strings.Join(lines, "\n")
}
