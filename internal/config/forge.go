// Forge mapping is declared, never guessed: ParseRemoteURL refuses unknown hosts on purpose, since an unknown host cannot say which provider it is or whether its URLs carry a subfolder prefix, and a guess yields links that 404 without failing visibly.
package config

import (
	"strings"

	"gitdash/internal/forge"
)

const DefaultGitLabAPIBase = "/api/v4/"

type ForgeConfig struct {
	Enabled bool
	// One host per provider on purpose: the door (gh/glab) is the same for every instance and the prefix comes from its api_base, so a host list would need a second datum nobody has.
	Host      string
	APIBase   string
	CloneBase string
}

// Returns no slashes, and without CloneBase derives the prefix with forge.PrefixFromAPIBase, which answers empty on an unexpected shape instead of guessing.
func (f ForgeConfig) ClonePrefix() string {
	if base := strings.Trim(strings.TrimSpace(f.CloneBase), "/"); base != "" {
		return base
	}
	return forge.PrefixFromAPIBase(f.APIBase)
}

// Hosts come from the forge package table, so a new host there shows up here without touching this package.
func DefaultForges() map[string]ForgeConfig {
	out := make(map[string]ForgeConfig, 2)
	for host, name := range forge.PublicHosts() {
		out[name] = ForgeConfig{Enabled: true, Host: host}
	}
	gl := out[forge.ForgeGitLab]
	gl.APIBase = DefaultGitLabAPIBase
	out[forge.ForgeGitLab] = gl
	return out
}

// The list is the two forge constants, not a copy: a third door without its argv here would be an action that fails with no explanation.
func supportedForge(name string) bool {
	switch normalizeForgeName(name) {
	case forge.ForgeGitHub, forge.ForgeGitLab:
		return true
	}
	return false
}

// Provider names are lowercased: they come from a hand-written config key, and a "GitLab" that missed the constant would leave the action with no door.
func (c *Config) addForge(rawName string, f forgeConfig) {
	name := normalizeForgeName(rawName)
	if name == "" {
		return
	}
	cur := c.Forges[name]
	if f.Enabled != nil {
		cur.Enabled = *f.Enabled
	}
	if f.Host != nil {
		if h := normalizeHost(*f.Host); h != "" {
			cur.Host = h
		}
	}
	if f.APIBase != nil && strings.TrimSpace(*f.APIBase) != "" {
		cur.APIBase = strings.TrimSpace(*f.APIBase)
	}
	if f.CloneBase != nil && strings.TrimSpace(*f.CloneBase) != "" {
		cur.CloneBase = strings.TrimSpace(*f.CloneBase)
	}
	c.Forges[name] = cur
}

// A disabled provider contributes no host, so its remotes fall back to "unknown forge" instead of waiting at a switched-off door.
func (c Config) ForgeHosts() map[string]string {
	out := make(map[string]string, len(c.Forges))
	for name, f := range c.Forges {
		if !f.Enabled {
			continue
		}
		if h := normalizeHost(f.Host); h != "" {
			out[h] = name
		}
	}
	return out
}

// The prefix comes from the same datum as the host (not a separate declaration): a wrong prefix does not break creation but does break the web URL.
func (c Config) ForgePrefixes() map[string]string {
	out := make(map[string]string, len(c.Forges))
	for _, f := range c.Forges {
		if !f.Enabled {
			continue
		}
		h := normalizeHost(f.Host)
		if h == "" {
			continue
		}
		if p := f.ClonePrefix(); p != "" {
			out[h] = p
		}
	}
	return out
}

func normalizeForgeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
