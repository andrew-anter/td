package gitlab

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"todo/task"
)

// Settings is the optional GitLab integration, stored in td's config
// file:
//
//	gitlab_url: https://gitlab.example.com
//	gitlab_token: glpat-…
//	gitlab_projects:
//	  gym: group/generator/gym
//
// TODO_GITLAB_URL and TODO_GITLAB_TOKEN override the file, so a token
// can stay out of disk entirely.
type Settings struct {
	// URL is the instance root, e.g. https://gitlab.example.com; a
	// subpath deployment (https://host/gitlab) is fine.
	URL string
	// Token is a personal access token with api scope.
	Token string
	// Links maps a td project name to a GitLab project path. Keys keep
	// their case: td project names are case-sensitive.
	Links map[string]string
}

// Configured reports whether a sync has everything it needs.
func (s Settings) Configured() bool {
	return strings.TrimSpace(s.URL) != "" && strings.TrimSpace(s.Token) != ""
}

// Link returns the GitLab project linked to the td project.
func (s Settings) Link(tdProject string) (string, bool) {
	p, ok := s.Links[tdProject]
	return p, ok && p != ""
}

// LinkedTo returns the td project already mirroring glPath, ignoring
// the project named except. One GitLab project backs at most one td
// project, or imports and state pushes would be duplicated.
func (s Settings) LinkedTo(glPath, except string) (string, bool) {
	for name, p := range s.Links {
		if name != except && p == glPath {
			return name, true
		}
	}
	return "", false
}

// ValidateURL checks that raw is an absolute http(s) URL.
func ValidateURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("gitlab url is not configured")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("gitlab url %q must be an http(s) base URL", raw)
	}
	return nil
}

// NormalizeProjectPath trims the whitespace and slashes around a GitLab
// project path.
func NormalizeProjectPath(p string) string {
	return strings.Trim(strings.TrimSpace(p), "/")
}

// ValidateProjectPath checks a GitLab project path: "name" for a
// user-level project or "group/sub/name". Spaces and control
// characters would have to be percent-encoded and are almost always a
// typo, so they are refused.
func ValidateProjectPath(p string) error {
	if NormalizeProjectPath(p) == "" {
		return fmt.Errorf("gitlab project path is not configured")
	}
	for _, r := range NormalizeProjectPath(p) {
		if r <= ' ' || r == 0x7f {
			return fmt.Errorf("gitlab project path %q must not contain spaces or control characters", p)
		}
	}
	return nil
}

// Env reports which settings the environment overrides. The config
// page shows file values and marks the overridden ones, so a token
// exported in the shell never gets written to disk behind the user's
// back.
type Env struct {
	URL   bool
	Token bool
}

// EnvOverrides reports which values TODO_GITLAB_URL and
// TODO_GITLAB_TOKEN currently supply.
func EnvOverrides() Env {
	return Env{
		URL:   strings.TrimSpace(os.Getenv("TODO_GITLAB_URL")) != "",
		Token: strings.TrimSpace(os.Getenv("TODO_GITLAB_TOKEN")) != "",
	}
}

// WithEnv layers the environment overrides on top of s.
func (s Settings) WithEnv() Settings {
	if v := strings.TrimSpace(os.Getenv("TODO_GITLAB_URL")); v != "" {
		s.URL = v
	}
	if v := strings.TrimSpace(os.Getenv("TODO_GITLAB_TOKEN")); v != "" {
		s.Token = v
	}
	return s
}

// LoadFileSettings reads just what the config file at path holds.
// Callers that write settings use this, so environment values are
// never persisted.
func LoadFileSettings(path string) Settings {
	s := Settings{Links: map[string]string{}}
	if b, err := os.ReadFile(path); err == nil {
		var doc yaml.Node
		if err := yaml.Unmarshal(b, &doc); err == nil {
			readSettings(&doc, &s)
		}
	}
	return s
}

// LoadSettings reads the config file at path and layers the
// environment overrides on top. A missing or unparsable file yields
// empty settings: td must stay usable when the integration is broken.
func LoadSettings(path string) Settings {
	return LoadFileSettings(path).WithEnv()
}

// SaveSettings writes url, token, and links into the config file at
// path, leaving every other key and all comments alone. Empty values
// are removed rather than written blank.
func SaveSettings(path string, s Settings) error {
	if path == "" {
		return fmt.Errorf("no config file to save to")
	}
	doc, err := readDoc(path)
	if err != nil {
		return err
	}
	m := ensureMapping(doc)
	setScalar(m, "gitlab_url", strings.TrimSpace(s.URL))
	setScalar(m, "gitlab_token", strings.TrimSpace(s.Token))
	setStringMap(m, "gitlab_projects", s.Links)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return task.WriteFileAtomic(path, buf.Bytes())
}

func readSettings(doc *yaml.Node, s *Settings) {
	m := mappingRoot(doc)
	if m == nil {
		return
	}
	if v := mappingValue(m, "gitlab_url"); v != nil {
		s.URL = strings.TrimSpace(v.Value)
	}
	if v := mappingValue(m, "gitlab_token"); v != nil {
		s.Token = strings.TrimSpace(v.Value)
	}
	if v := mappingValue(m, "gitlab_projects"); v != nil && v.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(v.Content); i += 2 {
			name := v.Content[i].Value
			path := NormalizeProjectPath(v.Content[i+1].Value)
			if name != "" && path != "" {
				s.Links[name] = path
			}
		}
	}
}

func readDoc(path string) (*yaml.Node, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyDoc(), nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return emptyDoc(), nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// A comment-only file parses to nothing; a "null" document is the
	// same as an empty one for our purposes.
	if doc.Kind == 0 {
		return emptyDoc(), nil
	}
	if root := mappingRoot(&doc); root == nil && doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 &&
		doc.Content[0].Kind == yaml.ScalarNode && doc.Content[0].Tag == "!!null" {
		return emptyDoc(), nil
	}
	if mappingRoot(&doc) == nil {
		return nil, fmt.Errorf("%s: top level must be a mapping", path)
	}
	return &doc, nil
}

func emptyDoc() *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{
		Kind: yaml.MappingNode, Tag: "!!map",
	}}}
}

// mappingRoot returns the document's top-level mapping, if any.
func mappingRoot(doc *yaml.Node) *yaml.Node {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	if root := doc.Content[0]; root.Kind == yaml.MappingNode {
		return root
	}
	return nil
}

// ensureMapping returns the document's top-level mapping, replacing a
// scalar (e.g. a file holding just "null") with an empty one.
func ensureMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind != yaml.DocumentNode {
		*doc = *emptyDoc()
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		*doc.Content[0] = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	return doc.Content[0]
}

// keyIndex returns the index of key in the mapping's alternating
// key/value content, or -1.
func keyIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if i := keyIndex(m, key); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}

// setScalar sets key to value, keeping the key node (and any comment
// attached to it); an empty value removes the key entirely.
func setScalar(m *yaml.Node, key, value string) {
	i := keyIndex(m, key)
	if value == "" {
		if i >= 0 {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
		}
		return
	}
	v := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if i >= 0 {
		m.Content[i+1] = v
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
}

// setStringMap writes key as a mapping with sorted keys, or removes it
// when values is empty.
func setStringMap(m *yaml.Node, key string, values map[string]string) {
	i := keyIndex(m, key)
	names := make([]string, 0, len(values))
	for n, v := range values {
		if n != "" && v != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		if i >= 0 {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
		}
		return
	}
	sort.Strings(names)

	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, n := range names {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: n},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: values[n]})
	}
	if i >= 0 {
		m.Content[i+1] = node
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, node)
}
