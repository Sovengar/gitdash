// Package forge resolves a git remote to its forge, pure by contract: an unconfigured host is not normalized, because an unknown host cannot say which provider it is or whether its URLs carry a prefix.
package forge

import (
	"strings"
)

const (
	ForgeGitHub = "github"
	ForgeGitLab = "gitlab"
)

type RepoRef struct {
	Forge   string
	Host    string
	Project string
	Owner   string
	Name    string
}

// Self-managed instances are absent on purpose: they come from the config, which is why ForgeForHost answers false instead of guessing.
var publicHosts = map[string]string{
	"github.com": ForgeGitHub,
	"gitlab.com": ForgeGitLab,
}

func ForgeForHost(host string) (string, bool) {
	forge, ok := publicHosts[normalizeHost(host)]
	return forge, ok
}

// A copy, and not the whole table, because the caller merges it with what the user declared, so a host added in one place cannot go missing in the other.
func PublicHosts() map[string]string {
	out := make(map[string]string, len(publicHosts))
	for host, forge := range publicHosts {
		out[host] = forge
	}
	return out
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}
