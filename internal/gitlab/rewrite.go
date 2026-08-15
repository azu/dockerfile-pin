package gitlab

import "strings"

// RewriteFile applies digests to a GitLab CI file, returning the modified content.
// Keys of digests are indices into refs.
func RewriteFile(content string, refs []GitLabImageRef, digests map[int]string) string {
	lines := strings.Split(content, "\n")
	for i, ref := range refs {
		digest, ok := digests[i]
		if !ok || ref.Skip {
			continue
		}
		lineIdx := ref.Line - 1
		if lineIdx < 0 || lineIdx >= len(lines) {
			continue
		}
		lines[lineIdx] = strings.Replace(lines[lineIdx], ref.RawRef, ref.ImageRef+"@"+digest, 1)
	}
	return strings.Join(lines, "\n")
}
