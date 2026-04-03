package actions

import "strings"

// RewriteFile applies digests to a GitHub Actions file, returning the modified content.
func RewriteFile(content string, refs []ActionsImageRef, digests map[int]string) string {
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
		oldValue := ref.RawRef
		var newValue string
		if ref.HasPrefix {
			// docker://image:tag -> docker://image:tag@sha256:...
			imageStr := strings.TrimPrefix(oldValue, "docker://")
			if atIdx := strings.Index(imageStr, "@"); atIdx >= 0 {
				imageStr = imageStr[:atIdx]
			}
			newValue = "docker://" + imageStr + "@" + digest
		} else {
			// image:tag -> image:tag@sha256:...
			imageStr := oldValue
			if atIdx := strings.Index(imageStr, "@"); atIdx >= 0 {
				imageStr = imageStr[:atIdx]
			}
			newValue = imageStr + "@" + digest
		}
		lines[lineIdx] = strings.Replace(lines[lineIdx], oldValue, newValue, 1)
	}
	return strings.Join(lines, "\n")
}
