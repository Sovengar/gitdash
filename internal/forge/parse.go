package forge

import (
	"net/url"
	"strings"
)

// hosts maps host → forge, answering false for unknown hosts on purpose so "unknown to me" stays distinct from "a local path"; prefixes strips the instance root from the path so one instance yields one project.
func ParseRemoteURL(raw string, hosts, prefixes map[string]string) (RepoRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RepoRef{}, false
	}

	var host, path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return RepoRef{}, false
		}
		// Hostname() and not Host: the port is not part of the identity and would break both the host lookup and the web URL.
		host = u.Hostname()
		path = u.Path
	} else if at := strings.Index(raw, "@"); at >= 0 {
		rest := raw[at+1:]
		colon := strings.Index(rest, ":")
		// `colon < 1`, not `colon < 0`: a hostless remote resolves through the map and leaves `-R owner/repo` in the `gh pr create` argv, a PR against a repo that does not know who it is.
		if colon < 1 {
			return RepoRef{}, false
		}
		host, path = rest[:colon], rest[colon+1:]
	} else {
		return RepoRef{}, false
	}
	host = normalizeHost(host)

	path = strings.Trim(strings.TrimSuffix(strings.TrimSpace(path), ".git"), "/")
	prefix := strings.Trim(prefixes[host], "/")
	if prefix != "" {
		path = strings.TrimPrefix(path, prefix+"/")
	}
	parts := strings.Split(path, "/")
	forge, ok := hosts[host]
	if !ok || forge == "" || len(parts) < 2 {
		return RepoRef{}, false
	}
	for _, p := range parts {
		if p == "" {
			return RepoRef{}, false
		}
	}
	return RepoRef{
		Forge:   forge,
		Host:    host,
		Project: strings.Join(parts, "/"),
		Owner:   parts[len(parts)-2],
		Name:    parts[len(parts)-1],
	}, true
}

// Answers "" for an empty or unexpected api_base instead of guessing: an invented prefix sends every web URL to a path that does not exist.
func PrefixFromAPIBase(apiBase string) string {
	p := strings.Trim(strings.TrimSpace(apiBase), "/")
	const rest = "api/v4"
	if p == "" || strings.Contains(p, "://") {
		return ""
	}
	if p == rest {
		return ""
	}
	if strings.HasSuffix(p, "/"+rest) {
		return strings.TrimSuffix(p, "/"+rest)
	}
	return ""
}
