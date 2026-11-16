// Package yamlnode looks values up in a yaml.Node tree the way YAML resolves
// aliases and merge keys, without expanding the tree.
//
// A value found through an alias or a merge key is the anchored node itself,
// so its Line and Column are those of the text the file actually contains.
package yamlnode

import "gopkg.in/yaml.v3"

// mergeTag is the tag yaml.v3 gives to a plain `<<` key. A quoted "<<" is a string.
const mergeTag = "!!merge"

// Entry is one key of a mapping together with its value.
type Entry struct {
	Key   string
	Value *yaml.Node
}

// Resolve follows alias nodes to the node they name.
// It returns nil for nil and for an alias chain that never ends.
func Resolve(node *yaml.Node) *yaml.Node {
	seen := map[*yaml.Node]bool{}
	for node != nil && node.Kind == yaml.AliasNode {
		if seen[node] {
			return nil
		}
		seen[node] = true
		node = node.Alias
	}
	return node
}

// Lookup returns the resolved value of key in mapping, or nil when it is absent.
// A key written in the mapping wins over a merged one, and among merged
// mappings the earlier one wins, as the YAML merge key type defines.
// https://yaml.org/type/merge.html
func Lookup(mapping *yaml.Node, key string) *yaml.Node {
	return lookup(mapping, key, map[*yaml.Node]bool{})
}

func lookup(mapping *yaml.Node, key string, visited map[*yaml.Node]bool) *yaml.Node {
	mapping = Resolve(mapping)
	if mapping == nil || mapping.Kind != yaml.MappingNode || visited[mapping] {
		return nil
	}
	// An anchor can contain an alias to itself, so a mapping is entered once.
	visited[mapping] = true
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if !isMergeKey(k) && k.Value == key {
			return Resolve(mapping.Content[i+1])
		}
	}
	for _, merged := range mergedMappings(mapping) {
		if v := lookup(merged, key, visited); v != nil {
			return v
		}
	}
	return nil
}

// Entries returns the keys of mapping with their resolved values: the keys
// written in the mapping in order, then the merged keys it does not override.
func Entries(mapping *yaml.Node) []Entry {
	var entries []Entry
	collect(mapping, map[string]bool{}, map[*yaml.Node]bool{}, &entries)
	return entries
}

func collect(mapping *yaml.Node, seenKeys map[string]bool, visited map[*yaml.Node]bool, entries *[]Entry) {
	mapping = Resolve(mapping)
	if mapping == nil || mapping.Kind != yaml.MappingNode || visited[mapping] {
		return
	}
	visited[mapping] = true
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if isMergeKey(k) || seenKeys[k.Value] {
			continue
		}
		seenKeys[k.Value] = true
		*entries = append(*entries, Entry{Key: k.Value, Value: Resolve(mapping.Content[i+1])})
	}
	for _, merged := range mergedMappings(mapping) {
		collect(merged, seenKeys, visited, entries)
	}
}

// mergedMappings returns the nodes merged into mapping, in precedence order.
func mergedMappings(mapping *yaml.Node) []*yaml.Node {
	var merged []*yaml.Node
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if !isMergeKey(mapping.Content[i]) {
			continue
		}
		value := Resolve(mapping.Content[i+1])
		if value == nil {
			continue
		}
		if value.Kind == yaml.SequenceNode {
			merged = append(merged, value.Content...)
			continue
		}
		merged = append(merged, value)
	}
	return merged
}

func isMergeKey(key *yaml.Node) bool {
	return key.Kind == yaml.ScalarNode && key.Tag == mergeTag
}
