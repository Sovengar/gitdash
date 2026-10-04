package forge

import "testing"

func refGH(project string) RepoRef {
	return RepoRef{Forge: ForgeGitHub, Host: "github.com", Project: project, Owner: "acme", Name: "widget"}
}

func TestForgeForHost(t *testing.T) {
	cases := []struct {
		host string
		want string
		ok   bool
	}{
		{"github.com", ForgeGitHub, true},
		{"gitlab.com", ForgeGitLab, true},
		{"GitHub.com", ForgeGitHub, true},
		{"  github.com  ", ForgeGitHub, true},
		{"gitlab.example.com", "", false},
		{"bitbucket.org", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got, ok := ForgeForHost(tc.host)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("ForgeForHost(%q) = %q, %v; quiero %q, %v", tc.host, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// PublicHosts returns a COPY so the caller mixing it with what the user declared cannot change forge's recognition table on the way.
func TestPublicHostsIsCopy(t *testing.T) {
	hosts := PublicHosts()
	if len(hosts) != 2 {
		t.Fatalf("PublicHosts() = %v, want both public hosts", hosts)
	}
	delete(hosts, "github.com")
	if _, ok := PublicHosts()["github.com"]; !ok {
		t.Error("deleting from the copy removed the host from the table")
	}
	if _, ok := ForgeForHost("github.com"); !ok {
		t.Error("ForgeForHost stopped recognising github.com")
	}
}
