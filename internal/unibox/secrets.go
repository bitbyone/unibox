package unibox

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

// SecretStore keeps unibox's secrets where the platform keeps them, so the
// user sees, rotates and deletes them with the system's own tools. A value
// passes through unibox's memory on its way in and never anywhere else:
// not argv, a log, JSON, state or a profile. A profile names a secret as
// secret:<name>, and Command turns that into the host command the runtime
// runs for the value.
type SecretStore interface {
	Name() string
	Set(ctx context.Context, name string, value []byte) error
	Has(ctx context.Context, name string) (bool, error)
	Remove(ctx context.Context, name string) error
	List(ctx context.Context) ([]string, error)
	Command(name string) string
}

const SecretPrefix = "secret:"

// SecretNameField stands for a secret's name in a person's own command
// for secrets.
const SecretNameField = "{name}"

// PlatformSecrets is the store of the platform unibox runs on.
func PlatformSecrets() (SecretStore, error) {
	switch runtime.GOOS {
	case "darwin":
		return Keychain{}, nil
	case "linux":
		return SecretService{}, nil
	}
	return nil, fmt.Errorf("unibox keeps no secrets on %s; name a host command in the profile instead", runtime.GOOS)
}

// SecretRef says which secret a profile's command names, if it is a
// reference rather than a command.
func SecretRef(command string) (string, bool) {
	name, ok := strings.CutPrefix(command, SecretPrefix)
	return name, ok
}

// ResolveCommand turns a profile's command into the host command a runtime
// runs: a secret reference becomes the person's own command for secrets,
// via, with {name} in it, or else the platform's lookup; anything else is
// the profile's command and stays as it is.
func ResolveCommand(command, via string) (string, error) {
	name, ok := SecretRef(command)
	if !ok {
		return command, nil
	}
	if via != "" {
		return strings.ReplaceAll(via, SecretNameField, name), nil
	}
	store, err := PlatformSecrets()
	if err != nil {
		return "", err
	}
	return store.Command(name), nil
}

func toolPath(tool string) string {
	if p, err := exec.LookPath(tool); err == nil {
		return p
	}
	return tool
}

// Keychain is the macOS login keychain, through security(1). Items are
// generic passwords under the service unibox-<name>. The value goes in as
// hex on security's interactive stdin, never as an argument, where ps
// would show it.
type Keychain struct{}

func (Keychain) Name() string { return "the login keychain" }

func keychainService(name string) string { return "unibox-" + name }

func (k Keychain) Set(ctx context.Context, name string, value []byte) error {
	if err := ValidName(name); err != nil {
		return err
	}
	encoded := hex.EncodeToString(value)
	cmd := hostCommand(ctx, "security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -a unibox -s %s -l \"unibox %s\" -X %s\n", keychainService(name), name, encoded))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	// security -i may echo the line it could not run; the value is cut out
	// of whatever it says before it reaches an error.
	said := strings.TrimSpace(strings.ReplaceAll(out.String(), encoded, "<value>"))
	if err != nil {
		return fmt.Errorf("the keychain did not store %s: %s", name, said)
	}
	if ok, err := k.Has(ctx, name); err != nil || !ok {
		return fmt.Errorf("the keychain did not store %s; unlock the login keychain and retry: %s", name, said)
	}
	return nil
}

func (Keychain) Has(ctx context.Context, name string) (bool, error) {
	// Without -w security prints the item's attributes, not its value.
	cmd := hostCommand(ctx, "security", "find-generic-password", "-s", keychainService(name))
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if e, ok := err.(*exec.ExitError); ok && e.ExitCode() == 44 {
		return false, nil
	}
	return false, fmt.Errorf("cannot read the login keychain: %w", err)
}

func (Keychain) Remove(ctx context.Context, name string) error {
	if _, err := commandOutput(hostCommand(ctx, "security", "delete-generic-password", "-s", keychainService(name))); err != nil {
		return fmt.Errorf("the keychain did not remove %s: %w", name, err)
	}
	return nil
}

var keychainItem = regexp.MustCompile(`"svce"<blob>="unibox-([a-z0-9][a-z0-9-]*)"`)

func (Keychain) List(ctx context.Context) ([]string, error) {
	// dump-keychain without -d lists attributes alone.
	out, err := commandOutput(hostCommand(ctx, "security", "dump-keychain"))
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, m := range keychainItem.FindAllStringSubmatch(string(out), -1) {
		if !slices.Contains(names, m[1]) {
			names = append(names, m[1])
		}
	}
	slices.Sort(names)
	return names, nil
}

func (Keychain) Command(name string) string {
	return toolPath("security") + " find-generic-password -s " + keychainService(name) + " -w"
}

// SecretService is the freedesktop Secret Service on Linux - GNOME Keyring,
// KWallet - through secret-tool, which reads the value on stdin. Items carry
// the attributes service=unibox and name=<name>.
type SecretService struct{}

func (SecretService) Name() string { return "the Secret Service" }

func (s SecretService) Set(ctx context.Context, name string, value []byte) error {
	if err := ValidName(name); err != nil {
		return err
	}
	cmd := hostCommand(ctx, "secret-tool", "store", "--label", "unibox "+name, "service", "unibox", "name", name)
	cmd.Stdin = bytes.NewReader(value)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("the Secret Service did not store %s; is a keyring unlocked? %s", name, strings.TrimSpace(out.String()))
	}
	return nil
}

func (SecretService) Has(ctx context.Context, name string) (bool, error) {
	// lookup answers with the value; it is read and dropped, never kept.
	cmd := hostCommand(ctx, "secret-tool", "lookup", "service", "unibox", "name", name)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	found := out.Len() > 0
	out.Reset()
	if err == nil {
		return found, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, fmt.Errorf("cannot reach the Secret Service; install secret-tool (libsecret-tools): %w", err)
}

func (SecretService) Remove(ctx context.Context, name string) error {
	if _, err := commandOutput(hostCommand(ctx, "secret-tool", "clear", "service", "unibox", "name", name)); err != nil {
		return fmt.Errorf("the Secret Service did not remove %s: %w", name, err)
	}
	return nil
}

var secretServiceItem = regexp.MustCompile(`(?m)^attribute\.name = ([a-z0-9][a-z0-9-]*)$`)

func (SecretService) List(ctx context.Context) ([]string, error) {
	// search prints each item's value beside its attributes; only the
	// attribute lines are kept.
	out, err := commandOutput(hostCommand(ctx, "secret-tool", "search", "--all", "service", "unibox"))
	if err != nil {
		return []string{}, nil
	}
	names := []string{}
	for _, m := range secretServiceItem.FindAllStringSubmatch(string(out), -1) {
		if !slices.Contains(names, m[1]) {
			names = append(names, m[1])
		}
	}
	slices.Sort(names)
	return names, nil
}

func (SecretService) Command(name string) string {
	return toolPath("secret-tool") + " lookup service unibox name " + name
}
