package actions

import (
	"sort"
	"strings"
)

// RewriteFile applies digests to a GitHub Actions file, returning the modified content.
func RewriteFile(content string, refs []ActionsImageRef, digests map[int]string) string {
	lines := strings.Split(content, "\n")
	// A flow mapping writes several references on one line, and one may be a prefix
	// of another, so each is replaced at the column it was written at rather than at
	// the first place its text occurs. Taking the rightmost first keeps the columns
	// of the references still to come valid, since inserting a digest only shifts
	// what follows it.
	for _, i := range rightmostFirst(refs) {
		ref := refs[i]
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
		lines[lineIdx] = replaceAt(lines[lineIdx], ref.Column, oldValue, newValue)
	}
	return strings.Join(lines, "\n")
}

// rightmostFirst orders indices into refs by descending column. References on
// different lines are independent, so only their relative order within a line matters.
func rightmostFirst(refs []ActionsImageRef) []int {
	order := make([]int, len(refs))
	for i := range refs {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return refs[order[a]].Column > refs[order[b]].Column
	})
	return order
}

// replaceAt replaces the reference written at column, 1-based, leaving the rest of
// the line alone. The reference is searched for from that column rather than spliced
// at it, because a quoted scalar starts at its opening quote. A column of 0, meaning
// unknown, searches the whole line.
func replaceAt(line string, column int, rawRef string, newRef string) string {
	start := column - 1
	if start < 0 || start > len(line) {
		start = 0
	}
	idx := strings.Index(line[start:], rawRef)
	if idx < 0 {
		return line
	}
	idx += start
	return line[:idx] + newRef + line[idx+len(rawRef):]
}
