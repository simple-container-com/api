// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"bytes"
	"os"
	"sort"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// scopesIndent is the indentation scopes.yaml is written with.
const scopesIndent = 2

// Save writes scopes.yaml. scopes.yaml is a reviewed governance file, so an
// existing one is updated in place: its comments, scope order and recipient
// order stay, scopes and recipients that were added are appended, and ones that
// were removed disappear. A missing or empty file is written fresh, in a stable
// field order. An existing file Save cannot edit safely (unreadable, not a
// mapping, or built with anchors, aliases or merge keys) is an error, never
// overwritten.
func (s *Scopes) Save(path string) error {
	if s.SchemaVersion == 0 {
		s.SchemaVersion = CurrentScopesSchemaVersion
	}
	var doc yaml.Node
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to read %s", path)
	}
	if err == nil {
		if uErr := yaml.Unmarshal(existing, &doc); uErr != nil {
			return errors.Wrapf(uErr, "%s does not parse; fix it before changing scopes", path)
		}
	}
	switch {
	case doc.Kind == 0:
		doc = yaml.Node{}
		if err := doc.Encode(s); err != nil {
			return errors.Wrap(err, "failed to marshal scopes")
		}
	case doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode:
		return errors.Errorf("%s is not a YAML mapping; fix it before changing scopes", path)
	case usesReferences(doc.Content[0]):
		return errors.Errorf("%s uses YAML anchors, aliases or merge keys, which an in-place edit could silently change; expand them by hand first", path)
	default:
		if err := s.updateNode(doc.Content[0]); err != nil {
			return err
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(scopesIndent)
	if err := enc.Encode(&doc); err != nil {
		return errors.Wrap(err, "failed to marshal scopes")
	}
	if err := enc.Close(); err != nil {
		return errors.Wrap(err, "failed to marshal scopes")
	}
	if err := writeFileAtomic(path, buf.Bytes(), 0o644); err != nil {
		return errors.Wrapf(err, "failed to write %s", path)
	}
	return nil
}

// usesReferences reports whether any node under n is an anchor, an alias or a
// merge key: editing one place in such a document can change others.
func usesReferences(n *yaml.Node) bool {
	if n.Anchor != "" || n.Kind == yaml.AliasNode || (n.Kind == yaml.ScalarNode && n.Tag == "!!merge") {
		return true
	}
	for _, c := range n.Content {
		if usesReferences(c) {
			return true
		}
	}
	return false
}

// updateNode makes the mapping root of an existing scopes.yaml say what s says,
// touching only what differs.
func (s *Scopes) updateNode(root *yaml.Node) error {
	var version yaml.Node
	if err := version.Encode(s.SchemaVersion); err != nil {
		return errors.Wrap(err, "failed to marshal scopes")
	}
	setMapValue(root, "schemaVersion", &version)

	scopes := mapValue(root, "scopes")
	if scopes == nil || scopes.Kind != yaml.MappingNode {
		scopes = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMapValue(root, "scopes", scopes)
	}

	kept := make([]*yaml.Node, 0, len(scopes.Content))
	present := map[string]bool{}
	for i := 0; i+1 < len(scopes.Content); i += 2 {
		name := scopes.Content[i].Value
		scope, ok := s.Scopes[name]
		if !ok {
			continue
		}
		if err := updateScopeNode(scopes.Content[i+1], scope); err != nil {
			return err
		}
		kept = append(kept, scopes.Content[i], scopes.Content[i+1])
		present[name] = true
	}
	scopes.Content = kept

	added := make([]string, 0, len(s.Scopes))
	for name := range s.Scopes {
		if !present[name] {
			added = append(added, name)
		}
	}
	sort.Strings(added)
	for _, name := range added {
		var value yaml.Node
		if err := value.Encode(s.Scopes[name]); err != nil {
			return errors.Wrap(err, "failed to marshal scopes")
		}
		scopes.Content = append(scopes.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, &value)
	}
	return nil
}

// updateScopeNode updates one scope's mapping, keeping the comments and order
// of the recipients that stay.
func updateScopeNode(node *yaml.Node, scope Scope) error {
	if node.Kind != yaml.MappingNode {
		return node.Encode(scope)
	}
	if scope.Description == "" {
		deleteMapKey(node, "description")
	} else if d := mapValue(node, "description"); d != nil && d.Kind == yaml.ScalarNode {
		if d.Value != scope.Description {
			d.Value, d.Tag, d.Style = scope.Description, "!!str", 0
		}
	} else {
		setMapValue(node, "description", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: scope.Description})
	}

	recipients := mapValue(node, "recipients")
	if recipients == nil || recipients.Kind != yaml.SequenceNode {
		recipients = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		setMapValue(node, "recipients", recipients)
	}
	want := make(map[string]bool, len(scope.Recipients))
	for _, r := range scope.Recipients {
		want[r] = true
	}
	kept := make([]*yaml.Node, 0, len(recipients.Content)+len(scope.Recipients))
	have := map[string]bool{}
	for _, item := range recipients.Content {
		if item.Kind == yaml.ScalarNode && want[item.Value] && !have[item.Value] {
			kept = append(kept, item)
			have[item.Value] = true
		}
	}
	for _, r := range scope.Recipients {
		if !have[r] {
			kept = append(kept, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: r})
			have[r] = true
		}
	}
	recipients.Content = kept
	recipients.Style = 0
	return nil
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func setMapValue(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			value.HeadComment, value.LineComment, value.FootComment = m.Content[i+1].HeadComment, m.Content[i+1].LineComment, m.Content[i+1].FootComment
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func deleteMapKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}
