// Package api is the versioned command-line contract shared with unibox clients.
package api

import "time"

const Version = 2

type Repository struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Branch   string `json:"branch"`
	HostPath string `json:"host_path,omitempty"`
	Author   string `json:"author"`
	// CoAuthor says whether the agent's commits name it as co-author;
	// unset means they do.
	CoAuthor *bool `json:"co_author,omitempty"`
	// Mounted is HostPath mounted into the sandbox at the repository's
	// place in the task instead of cloned.
	Mounted bool `json:"mounted,omitempty"`
	// Context is a repository the task reads, which its agents are told to
	// leave as it is.
	Context bool `json:"context,omitempty"`
	// Base is the branch a clone's Branch is made from in the sandbox when
	// the forge has none: a branch of the task's alone, which unibox never
	// pushes.
	Base string `json:"base,omitempty"`
}

type Task struct {
	ID string `json:"id"`
	// Alias is a name the user gave the task, taken wherever its ID is;
	// its directory keeps the ID.
	Alias        string       `json:"alias,omitempty"`
	Sandbox      string       `json:"sandbox"`
	Directory    string       `json:"directory"`
	Repositories []Repository `json:"repositories"`
	Agent        string       `json:"agent,omitempty"`
	State        string       `json:"state"`
}

type Run struct {
	ID      string    `json:"id"`
	Sandbox string    `json:"sandbox"`
	Task    string    `json:"task,omitempty"`
	Agent   string    `json:"agent,omitempty"`
	PID     int       `json:"pid"`
	Pane    string    `json:"pane,omitempty"`
	Socket  string    `json:"socket,omitempty"`
	Since   time.Time `json:"since"`
}

type Sandbox struct {
	// Name is the sandbox's name in sbx as well.
	Name string `json:"name"`
	// ID is sbx's for it, so a sandbox replaced outside unibox is told.
	ID string `json:"id,omitempty"`
	// Env is the env the sandbox was created from, Files its sbxenv files
	// and Fingerprint what they, their local kits and the person's args
	// were then, so a change can be told.
	Env         string   `json:"env"`
	Files       []string `json:"files"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	// Agent is the agent the env's workload kit runs, and AgentCommand
	// how the kit starts it: its entrypoint and default arguments.
	Agent        string   `json:"agent,omitempty"`
	AgentCommand []string `json:"agent_command,omitempty"`
	// Agents is every agent a workload kit of the env runs, with how; a
	// task may run any of them, or another its sandbox has.
	Agents map[string][]string `json:"agents,omitempty"`
	// Adopted says unibox took over a sandbox it did not create, which it
	// then removes only when forced.
	Adopted bool   `json:"adopted,omitempty"`
	State   string `json:"state"`
	Tasks   []Task `json:"tasks"`
	Runs    []Run  `json:"runs"`
	// Ports are what unibox published of the sandbox, by name - an env's
	// port, or a spec switched on for the sandbox alone - and PortSwitches
	// what was switched for it until it next starts; PortMaps the mapping
	// a named port was switched on with in place of its own, as long.
	Ports        map[string]string `json:"ports,omitempty"`
	PortSwitches map[string]bool   `json:"port_switches,omitempty"`
	PortMaps     map[string]string `json:"port_maps,omitempty"`
}

// PublishedPort is a sandbox port published on the host, as the runtime
// lists it.
type PublishedPort struct {
	HostIP      string `json:"host_ip"`
	HostPort    int    `json:"host_port"`
	SandboxPort int    `json:"sandbox_port"`
	Protocol    string `json:"protocol"`
}

// Port is one of a sandbox's ports, as unibox port ls lists it.
// Team is whether the env's team named it, and Description the team's
// word for what listens there.
type Port struct {
	Name        string `json:"name"`
	Port        string `json:"port"`
	Description string `json:"description,omitempty"`
	Team        bool   `json:"team,omitempty"`
	// From is where the port is named: repo, env, yours-all or yours.
	From    string `json:"from,omitempty"`
	Default bool   `json:"default"`
	// DefaultYours says your config.yaml decides Default.
	DefaultYours bool            `json:"default_yours,omitempty"`
	Named        bool            `json:"named"`
	On           bool            `json:"on"`
	Published    []PublishedPort `json:"published"`
}

// NetworkRule is a host a sandbox may or may not reach, as unibox network
// ls lists it. From is yours (kept by unibox), sbx (added outside unibox),
// env (the env's kits) or default (sbx's own); Sandbox is empty for a rule
// every sandbox has. Applied says sbx has a rule of yours, which it cannot
// while the sandbox is not created.
type NetworkRule struct {
	Decision string `json:"decision"`
	Host     string `json:"host"`
	Protocol string `json:"protocol"`
	From     string `json:"from"`
	Sandbox  string `json:"sandbox,omitempty"`
	Applied  bool   `json:"applied"`
}

// BlockedHost is a host the policy refused a sandbox lately: what its env
// leaves out, unless it is to stay out.
type BlockedHost struct {
	Host     string    `json:"host"`
	Reason   string    `json:"reason"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

type Need struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Repository string `json:"repository,omitempty"`
	Agent      string `json:"agent,omitempty"`
	Warning    bool   `json:"warning,omitempty"`
}

type Check struct {
	APIVersion   int          `json:"api_version"`
	Ready        bool         `json:"ready"`
	Needs        []Need       `json:"needs"`
	Repositories []Repository `json:"repositories"`
}

type Prepared struct {
	APIVersion int    `json:"api_version"`
	Sandbox    string `json:"sandbox"`
	Task       string `json:"task"`
	Directory  string `json:"directory"`
	State      string `json:"state"`
}

type Error struct {
	APIVersion int    `json:"api_version"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}
