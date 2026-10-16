// Package api is the versioned command-line contract shared with unibox clients.
package api

import (
	"encoding/json"
	"time"
)

const Version = 1

type Repository struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Branch   string `json:"branch"`
	HostPath string `json:"host_path,omitempty"`
	Author   string `json:"author"`
	// CoAuthor is a git service's say over the agent's co-author trailer
	// for this repository; unset leaves it to the profile.
	CoAuthor *bool `json:"co_author,omitempty"`
	// Mounted is HostPath mounted into the sandbox at the repository's
	// place in the task instead of cloned.
	Mounted bool `json:"mounted,omitempty"`
}

type Task struct {
	ID           string       `json:"id"`
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
	Name                string   `json:"name"`
	Runtime             string   `json:"runtime"`
	RuntimeName         string   `json:"runtime_name"`
	RuntimeID           string   `json:"runtime_id,omitempty"`
	Profile             string   `json:"profile"`
	Instructions        []string `json:"instructions"`
	Revision            string   `json:"revision,omitempty"`
	InstructionRevision string   `json:"instruction_revision,omitempty"`
	State               string   `json:"state"`
	Memory              string   `json:"memory,omitempty"`
	Tasks               []Task   `json:"tasks"`
	Runs                []Run    `json:"runs"`
	// Image and Settings are what the sandbox was created from, so a
	// recreation can say what changed. NetworkAllow and NetworkDeny are
	// the rules unibox applied to it; a change of them is applied live.
	Image        string          `json:"image,omitempty"`
	Settings     json.RawMessage `json:"settings,omitempty"`
	NetworkAllow []string        `json:"network_allow,omitempty"`
	NetworkDeny  []string        `json:"network_deny,omitempty"`
	// Ports are what unibox published of the sandbox, by name - a profile's
	// port, or a spec switched on for the sandbox alone - and PortSwitches
	// what was switched for it until it next starts.
	Ports        map[string]string `json:"ports,omitempty"`
	PortSwitches map[string]bool   `json:"port_switches,omitempty"`
}

// PublishedPort is a sandbox port published on the host, as the runtime
// lists it.
type PublishedPort struct {
	HostIP      string `json:"host_ip"`
	HostPort    int    `json:"host_port"`
	SandboxPort int    `json:"sandbox_port"`
	Protocol    string `json:"protocol"`
}

// Port is one of a sandbox's ports, as unibox port lists it.
type Port struct {
	Name      string          `json:"name"`
	Port      string          `json:"port"`
	Default   bool            `json:"default"`
	Named     bool            `json:"named"`
	On        bool            `json:"on"`
	Published []PublishedPort `json:"published"`
}

type Profile struct {
	Name       string            `json:"name"`
	Agents     []string          `json:"agents"`
	Revision   string            `json:"revision,omitempty"`
	Images     map[string]string `json:"images"`
	Approved   bool              `json:"approved"`
	Error      string            `json:"error,omitempty"`
	LastCommit string            `json:"last_commit,omitempty"`
}

type Need struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Repository string `json:"repository,omitempty"`
	Agent      string `json:"agent,omitempty"`
	Service    string `json:"service,omitempty"`
	Warning    bool   `json:"warning,omitempty"`
}

type Check struct {
	APIVersion   int          `json:"api_version"`
	Ready        bool         `json:"ready"`
	Needs        []Need       `json:"needs"`
	Repositories []Repository `json:"repositories"`
}

type Prepared struct {
	APIVersion  int    `json:"api_version"`
	Sandbox     string `json:"sandbox"`
	Runtime     string `json:"runtime"`
	RuntimeName string `json:"runtime_name"`
	Task        string `json:"task"`
	Directory   string `json:"directory"`
	State       string `json:"state"`
}

type Error struct {
	APIVersion int    `json:"api_version"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}
