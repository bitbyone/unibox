package unibox

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:scaffold
var scaffoldFS embed.FS

// InitReport says what Init wrote and what it found already there and kept.
type InitReport struct {
	Root          string
	Written, Kept []string
}

// Init lays the environments repository's mandatory structure into root and,
// given a name, adds one profile: the shared runner and its installation
// scripts, the unibox instructions text, and the profile's own files with the
// chosen agents installed. It writes only what is missing - a file already
// there is kept word for word - so it is safe on a repository in use.
func Init(root, name string, agents []string, cpus int, memory string) (InitReport, error) {
	report := InitReport{Root: root}
	if name != "" {
		if err := ValidName(name); err != nil {
			return report, err
		}
		if len(agents) == 0 {
			return report, fmt.Errorf("choose at least one agent for the profile")
		}
		for _, id := range agents {
			if _, err := FindAgent(id); err != nil {
				return report, err
			}
		}
		if memory != "" && !memoryPattern.MatchString(memory) {
			return report, fmt.Errorf("invalid memory %q; use e.g. 8g", memory)
		}
	}
	place := func(rel string, data []byte, mode os.FileMode) error {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Lstat(path); err == nil {
			report.Kept = append(report.Kept, rel)
			return nil
		}
		if err := AtomicWrite(path, data, mode); err != nil {
			return err
		}
		report.Written = append(report.Written, rel)
		return nil
	}
	err := fs.WalkDir(scaffoldFS, "scaffold", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(path, "scaffold/")
		dest := rel
		if rel == ".install" || strings.HasPrefix(rel, "installations/") {
			dest = "_shared/" + rel
		}
		data, err := scaffoldFS.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if rel == ".install" || strings.HasSuffix(rel, ".sh") {
			mode = 0o755
		}
		return place(dest, data, mode)
	})
	if err != nil {
		return report, err
	}
	if name == "" {
		if err := os.MkdirAll(filepath.Join(root, "sandboxes"), 0o755); err != nil {
			return report, err
		}
		return report, CheckLayout(root)
	}
	dir := "sandboxes/" + name
	lines := []string{"base", "tools/nvm"}
	declared := []string{}
	for _, id := range agents {
		lines = append(lines, "agents/"+id)
		declared = append(declared, id)
	}
	yaml := fmt.Sprintf(`# %s: describe the environment here.
dockerfile: Dockerfile
cpus: %d
memory: %s
agents: [%s]
instructions: [unibox]
# How each agent signs in: unibox login %[1]s writes this, keeping each token
# in the platform's store as secret:<name>. sbx signs one agent of a sandbox
# in by OAuth.
# auth:
#   codex: oauth                      # the ChatGPT login sbx keeps
#   claude: {token: secret:claude}    # from claude setup-token
#   copilot: {token: secret:copilot}  # a GitHub token for copilot alone
# Host-facing services the sandbox reaches: unibox service add %[1]s <name>
# writes these. The token lives on the host and reaches the sandbox only as a
# placeholder the proxy swaps. A git service wires the sandbox's git; an env
# service names the variable a tool reads; command: is secret:<name> or any
# host command (vault, op) for a store unibox does not keep.
# services:
#   github:                           # a git forge
#     git: true
#     host: github.com
#     scheme: basic                   # Authorization: Basic base64(user:token)
#     user: x-access-token
#     command: secret:github
#     refresh: 10m                    # optional: fetch again, for short-lived tokens
#   github-acme:                      # another owner on the same forge
#     git: true
#     host: github.com/acme           # a path: its own token for these repositories
#     user: x-access-token
#     command: secret:github-acme
#     author: Jan Novak <jan@acme.example>   # who commits under it, unless --author says otherwise
#     co_author: false                       # no agent trailer in these repositories
#   artifactory:                      # any host service
#     host: [artifactory.example.com, "*.jfrog.io"]
#     scheme: bearer
#     env: ARTIFACTORY_TOKEN          # the tool reads the placeholder here
#     command: op read op://vault/artifactory/token
# Egress beyond the runtime's policy, and hosts to refuse; deny wins.
# allow_network: [jitpack.io]
# deny_network: [gist.github.com]
`, name, cpus, memory, strings.Join(declared, ", "))
	dockerfile := fmt.Sprintf(`# The image is composed by _shared/install: list the installation scripts,
# one per line. The sbx build passes BASE=docker/sandbox-templates:shell
# (.sbx/build.args); the default keeps this buildable by hand.
ARG BASE=ubuntu:24.04
FROM ${BASE}

ARG UNIBOX_RUNTIME
USER root

RUN --mount=type=bind,source=_shared,target=/unibox /unibox/.install <<EOF
%s
EOF

# The shell template ships an agent user; a plain ubuntu base needs one made.
RUN id agent >/dev/null 2>&1 || useradd -m -s /bin/bash agent
USER agent
WORKDIR /home/agent
`, strings.Join(lines, "\n"))
	instructions := fmt.Sprintf("# %s\n\nWhat the agent should know when it works in this profile.\n", name)
	for _, f := range []struct {
		rel  string
		data string
		mode os.FileMode
	}{
		{dir + "/sandbox.yaml", yaml, 0o644},
		{dir + "/Dockerfile", dockerfile, 0o644},
		{dir + "/instructions.md", instructions, 0o644},
		{dir + "/.sbx/build.args", "BASE=docker/sandbox-templates:shell\n", 0o644},
	} {
		if err := place(f.rel, []byte(f.data), f.mode); err != nil {
			return report, err
		}
	}
	if _, err := LoadProfile(root, name); err != nil {
		return report, fmt.Errorf("the repository did not come out loadable: %w", err)
	}
	return report, nil
}
