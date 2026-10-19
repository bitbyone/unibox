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
	// inline is a section whose example stands where the section would,
	// above the next one there, not in the tail.
	inline bool
}

// profileSections is the order of a profile's top-level keys, with what
// each says above it and how it reads when absent. %s is the profile.
var profileSections = []profileSection{
	{key: "version"},
	{key: "template", comment: "# Copied from this template's profile; unibox template update|diff tells\n# what changed there since."},
	{key: "image"},
	{key: "resources", comment: "# What a sandbox gets: CPUs and memory, each fixed (4, 8g) or a share of\n# the host's (50%). sbx gives at most 75% of the host's memory.", example: "# resources: {cpus: 50%, memory: 8g}  # fixed, or a share of the host's"},
	{key: "vars", inline: true, comment: "# What each person sets for themselves, with what it is for: unibox var set\n# <name> <value> keeps it in their ~/.config/unibox/vars.yaml, never here, and a\n# run stops while one it refers to is missing. Referred to as ${vars.<name>}.", example: "# vars:                               # each person's own: unibox var set <name> <value>\n#   artifactory-user: your Artifactory user name"},
	{key: "env", inline: true, comment: "# Variables of every session and of start: a host variable by name, or\n# NAME: value, which may refer to ${vars.<name>} or ${services.<name>}.", example: "# env:                                # every session's variables\n#   - BUILD_MODE                      # from the host\n#   - GRADLE_USER: ${vars.artifactory-user}\n#   - ARTIFACTORY_TOKEN: ${services.artifactory}  # the service's placeholder"},
	{key: "agents", comment: "# The agents the image has, and for each: how it signs in (auth - unibox\n# agent login; secret:<name> is a token unibox keeps), how it is credited in\n# commits (co_author - unibox agent co-author; Name <> gives no address),\n# and the seeds of its own configuration files, merged before every session."},
	{key: "instructions"},
	{key: "start", comment: "# Runs in the VM once per boot.", example: "# start: start.sh                     # in the VM, once per boot"},
	{key: "scripts", comment: "# Run in a sandbox only when asked for, by name: unibox run|resume\n# --script <name> before the agent starts, unibox script <sandbox> <name> now.", example: "# scripts:                            # run when asked: --script <name>, unibox script\n#   update-skills: update-skills.sh"},
	{key: "git", comment: "# Who commits: you, from the host's git (--author on a run wins)."},
	{key: "services", comment: "# Forges and services the sandbox reaches - unibox service add. The token\n# stays on the host; the sandbox sees a placeholder the proxy swaps on\n# requests to the service's hosts. git signs in through a git service by\n# itself; a tool finds another's placeholder in env as ${services.<name>}. A\n# service also takes refresh, author and co_author.", example: "# services:                           # unibox service add %s <name>\n#   github: {git: true, host: github.com, user: x-access-token, command: secret:github}"},
	{key: "network", comment: "# What the sandbox may reach, and which of its ports the host reaches - all\n# applied live, no recreation:\n#   allow: [jitpack.io]          hosts beyond the runtime's policy\n#   deny: [gist.github.com]      hosts refused; deny wins over allow\n#   ports:                       sandbox ports on the host's localhost, by name\n#     web: '3000:3000'           host port 3000 to sandbox port 3000, at every start\n#     api: '8080'                sandbox port 8080 on any free host port\n#     devtools: {port: '9333:9222', default: off}   published only when switched on\n# unibox network allow|deny|port <profile> changes this; unibox port <sandbox>\n# lists the ports with their URLs, and unibox port <sandbox> web:off devtools:on\n# switches them while the sandbox runs, leaving this file as it is.", example: "# network:                            # unibox network allow|deny|port %s ...\n#   allow: [jitpack.io]               # hosts beyond the runtime's policy\n#   deny: [gist.github.com]           # hosts refused; deny wins over allow\n#   ports:                            # sandbox ports on the host's localhost, by name\n#     web: '3000:3000'                # host:sandbox, published at every start\n#     api: '8080'                     # any free host port; unibox port <sandbox> shows it\n#     devtools: {port: '9333:9222', default: off}  # only when switched on"},
	{key: "build_args", comment: "# Build arguments for every runtime; a value may refer to ${vars.<name>},\n# and stays in the image's history - never a secret.", example: "# build_args: {NPM_MIRROR: ${vars.npm-mirror}}"},
	{key: "caches", comment: "# Host directories mounted read-only, named inside by a variable.", example: "# caches:                             # host directories mounted read-only\n#   - {host: ~/.gradle/caches, env: GRADLE_RO_DEP_CACHE}"},
}

// formerComments are comments unibox wrote above a section before it said
// them as profileSections do now; they are unibox's, so replaced, not kept
// as the user's.
var formerComments = []string{
	"# What the sandbox reaches beyond the runtime's policy (allow), what it may\n# not (deny wins), and its ports on the host's loopback by name - a spec, or\n# {port, default: off} to publish only when asked. Applied live: unibox\n# network allow|deny|port; unibox port <sandbox> name:on|off for now.",
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
	for _, c := range formerComments {
		owned[c] = true
	}
	present := map[string]bool{}
	for _, p := range pairs {
		present[p.k.Value] = true
	}
	// An inline example written before is unibox's, and goes before the
	// comments are told apart.
	inlines := map[int]string{}
	for i, sec := range profileSections {
		if sec.inline && sec.example != "" {
			inlines[i] = strings.ReplaceAll(sec.example, "%s", name)
		}
	}
	for _, p := range pairs {
		for _, ex := range inlines {
			p.k.HeadComment = strings.ReplaceAll(p.k.HeadComment+"\n", ex+"\n", "")
		}
		p.k.HeadComment = strings.Trim(p.k.HeadComment, "\n")
	}
	placed := map[int]bool{}
	m.Content = m.Content[:0]
	previous := ""
	for _, p := range pairs {
		comment := ""
		at := index(p.k.Value)
		if at >= 0 {
			comment = profileSections[at].comment
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
		// The inline examples of absent sections before this one stand
		// above it.
		if at >= 0 {
			above := []string{}
			for i := 0; i < at; i++ {
				if ex, ok := inlines[i]; ok && !present[profileSections[i].key] && !placed[i] {
					above = append(above, ex)
					placed[i] = true
				}
			}
			if len(above) > 0 {
				p.k.HeadComment = strings.TrimRight(strings.Join(above, "\n")+"\n\n"+p.k.HeadComment, "\n")
			}
		}
		m.Content = append(m.Content, p.k, p.v)
	}
	examples := []string{}
	for i, s := range profileSections {
		if s.example != "" && !present[s.key] && !placed[i] {
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
