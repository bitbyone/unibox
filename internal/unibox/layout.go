package unibox

import (
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// The profile is laid out the same way whoever wrote it last: the sections
// it has in a fixed order, each under a short comment saying what it does
// and which command changes it, and below them, commented out, only the
// sections it does not have yet. The tail is recomputed on every change
// unibox makes, so a section unibox writes never lands under examples of
// itself. A comment the user wrote over a section is theirs and stays.

// footerMarker opens the commented tail unibox owns and rewrites.
const footerMarker = "# More a profile can say, commented out; unibox rewrites this part:"

type profileSection struct {
	key, comment, example string
}

// profileSections is the order of a profile's top-level keys, with what
// each says above it and how it reads when absent. %s is the profile.
var profileSections = []profileSection{
	{key: "version"},
	{key: "dockerfile"},
	{key: "image"},
	{key: "resources", comment: "# What a sandbox gets: CPUs and memory, each fixed (4, 8g) or a share of\n# the host's (50%). sbx gives at most 75% of the host's memory.", example: "# resources: {cpus: 50%, memory: 8g}  # fixed, or a share of the host's"},
	{key: "agents", comment: "# The agents the image has, and for each: how it signs in (auth - unibox\n# agent login; secret:<name> is a token unibox keeps), how it is credited in\n# commits (co_author - unibox agent co-author; Name <> gives no address),\n# and the seeds of its own configuration files, merged before every session."},
	{key: "instructions"},
	{key: "start", comment: "# Runs in the VM once per boot.", example: "# start: start.sh                     # in the VM, once per boot"},
	{key: "bootstrap", comment: "# Runs in each worked-on repository that has it.", example: "# bootstrap: .agent/bootstrap.sh      # per worked-on repository, if present"},
	{key: "git", comment: "# Who commits: you, from the host's git (--author on a run wins)."},
	{key: "services", comment: "# Forges and services the sandbox reaches - unibox service add. The token\n# stays on the host; the sandbox sees a placeholder the proxy swaps. A\n# service also takes refresh, author and co_author.", example: "# services:                           # unibox service add %s <name>\n#   github: {git: true, host: github.com, user: x-access-token, command: secret:github}"},
	{key: "network", comment: "# What the sandbox reaches beyond the runtime's policy (allow), what it may\n# not (deny wins), and its ports on the host's loopback by name - a spec, or\n# {port, default: off} to publish only when asked. Applied live: unibox\n# network allow|deny|port; unibox port <sandbox> name:on|off for now.", example: "# network:                            # unibox network allow|deny|port %s ...\n#   allow: [jitpack.io]\n#   deny: [gist.github.com]\n#   ports: {web: '3000:3000', devtools: {port: '9333:9222', default: off}}"},
	{key: "env", comment: "# Host variables passed in by name.", example: "# env: [BUILD_MODE]                   # host variables passed in by name"},
	{key: "caches", comment: "# Host directories mounted read-only, named inside by a variable.", example: "# caches:                             # host directories mounted read-only\n#   - {host: ~/.gradle/caches, env: GRADLE_RO_DEP_CACHE}"},
}

// ArrangeProfile lays a profile's top-level mapping out in the sections'
// order with their comments, and rewrites the commented tail of what it
// lacks.
func ArrangeProfile(doc *yaml.Node, name string) {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return
	}
	m := doc.Content[0]
	stripFooter(doc)
	index := func(key string) int {
		return slices.IndexFunc(profileSections, func(s profileSection) bool { return s.key == key })
	}
	type pair struct{ k, v *yaml.Node }
	pairs := []pair{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		pairs = append(pairs, pair{m.Content[i], m.Content[i+1]})
	}
	slices.SortStableFunc(pairs, func(a, b pair) int {
		x, y := index(a.k.Value), index(b.k.Value)
		if x < 0 {
			x = len(profileSections)
		}
		if y < 0 {
			y = len(profileSections)
		}
		return x - y
	})
	owned := map[string]bool{}
	for _, s := range profileSections {
		if s.comment != "" {
			owned[s.comment] = true
		}
	}
	present := map[string]bool{}
	m.Content = m.Content[:0]
	previous := ""
	for _, p := range pairs {
		present[p.k.Value] = true
		comment := ""
		if i := index(p.k.Value); i >= 0 {
			comment = profileSections[i].comment
		}
		if comment != "" && (p.k.HeadComment == "" || owned[p.k.HeadComment]) {
			// The network rules share one comment, said once.
			if comment == previous {
				p.k.HeadComment = ""
			} else {
				p.k.HeadComment = comment
			}
		}
		previous = comment
		m.Content = append(m.Content, p.k, p.v)
	}
	examples := []string{}
	for _, s := range profileSections {
		if s.example != "" && !present[s.key] {
			examples = append(examples, strings.ReplaceAll(s.example, "%s", name))
		}
	}
	if len(examples) > 0 {
		doc.FootComment = footerMarker + "\n" + strings.Join(examples, "\n")
	}
}

// stripFooter takes unibox's commented tail out of wherever the YAML parser
// hung it - the document, the mapping, or the last node down its end - and
// leaves any comment of the user's before it.
func stripFooter(n *yaml.Node) {
	cut := func(c string) string {
		if i := strings.Index(c, footerMarker); i >= 0 {
			return strings.TrimRight(c[:i], "\n")
		}
		return c
	}
	n.FootComment = cut(n.FootComment)
	n.HeadComment = cut(n.HeadComment)
	for _, c := range n.Content {
		stripFooter(c)
	}
}
