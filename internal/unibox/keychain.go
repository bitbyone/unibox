package unibox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The system's keychain keeps a token unibox gives one env or one sandbox,
// as sbx keeps its own: sbx forgets a sandbox's tokens at every recreate,
// so what it is given is a command that reads the keychain. Each system's
// own program asks for the value on the person's terminal and stores it,
// and another reads it for sbx: the value never passes through unibox.
// macOS has security and its login keychain, Linux secret-tool and the
// Secret Service (GNOME Keyring, KWallet), Windows PowerShell, which
// encrypts it for the person's account alone with DPAPI.

// keychainService is what every item unibox stores is filed under.
const keychainService = "unibox"

// KeychainItem names the item a scope's token of a service is: its scope
// and service, for a person to find it in their keychain's own tool.
func KeychainItem(s SecretScope, service string) string {
	if s.Sandbox != "" {
		return "sandbox/" + s.Sandbox + "/" + service
	}
	return "env/" + s.Env + "/" + service
}

// KeychainName is what the person knows the system's keychain as.
func KeychainName() string {
	switch runtime.GOOS {
	case "darwin":
		return "the macOS keychain"
	case "windows":
		return "a file only your Windows account can decrypt (DPAPI)"
	}
	return "the Secret Service keyring"
}

// KeychainAvailable says whether this machine has the program its
// keychain is reached with, and what to do when it has not.
func KeychainAvailable() error {
	name := map[string]string{"darwin": "security", "windows": "powershell"}[runtime.GOOS]
	if name == "" {
		name = "secret-tool"
	}
	if _, err := exec.LookPath(name); err != nil {
		if name == "secret-tool" {
			return fmt.Errorf("secret-tool is not installed; install libsecret-tools (Debian, Ubuntu) or libsecret (Fedora, Arch) with a keyring such as GNOME Keyring, or give the token's --command")
		}
		return fmt.Errorf("%s is not on PATH, so the keychain cannot be reached; give the token's --command", name)
	}
	return nil
}

// dpapiFile is where Windows keeps an item, encrypted for the person.
func (p Paths) dpapiFile(item string) string {
	return filepath.Join(p.Config, "secrets", strings.ReplaceAll(item, "/", "_")+".dpapi")
}

// psQuote is a PowerShell string of s.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// KeychainStore has the system's own program ask for an item's value on
// this terminal and keep it, over one it had. label says what it is in
// the keychain's own tool.
func (p Paths) KeychainStore(ctx context.Context, item, label string, in io.Reader, out, errout io.Writer) error {
	if err := KeychainAvailable(); err != nil {
		return err
	}
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// -w last has security ask for the value, twice.
		c = hostCommand(ctx, "security", "add-generic-password", "-U", "-s", keychainService, "-a", item, "-l", label, "-w")
	case "windows":
		file := p.dpapiFile(item)
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return err
		}
		// Read-Host asks only in an interactive PowerShell.
		c = hostCommand(ctx, "powershell", "-NoProfile", "-Command",
			"$v = Read-Host -AsSecureString "+psQuote(label)+"; if ($v.Length -eq 0) { exit 3 }; $v | ConvertFrom-SecureString | Set-Content -LiteralPath "+psQuote(file))
	default:
		c = hostCommand(ctx, "secret-tool", "store", "--label", label, keychainService, item)
		// The keyring is reached over the session's bus, and may ask to
		// be unlocked on its display.
		c.Env = append(c.Env, keyringEnvironment()...)
	}
	c.Stdin, c.Stdout, c.Stderr = in, out, errout
	if err := c.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && runtime.GOOS == "windows" && exit.ExitCode() == 3 {
			return fmt.Errorf("nothing was typed, so no token is kept; run it again and paste the token")
		}
		return fmt.Errorf("the keychain did not keep the token; %w", err)
	}
	return nil
}

// KeychainCommand is the command sbx runs on the host to read an item.
// It is run through a shell, so every part is quoted.
func (p Paths) KeychainCommand(item string) string {
	switch runtime.GOOS {
	case "darwin":
		return "security find-generic-password -s " + Quote(keychainService) + " -a " + Quote(item) + " -w"
	case "windows":
		// Double quotes hold the script for cmd and a POSIX shell alike;
		// the script has no $ a shell would expand.
		return `powershell -NoProfile -NonInteractive -Command "[Net.NetworkCredential]::new('', (Get-Content -LiteralPath ` + psQuote(p.dpapiFile(item)) + ` | ConvertTo-SecureString)).Password"`
	}
	return "secret-tool lookup " + Quote(keychainService) + " " + Quote(item)
}

// KeychainRemove takes an item out of the keychain; one already gone is
// no failure.
func (p Paths) KeychainRemove(ctx context.Context, item string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = hostCommand(ctx, "security", "delete-generic-password", "-s", keychainService, "-a", item)
	case "windows":
		if err := os.Remove(p.dpapiFile(item)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	default:
		c = hostCommand(ctx, "secret-tool", "clear", keychainService, item)
		c.Env = append(c.Env, keyringEnvironment()...)
	}
	if c.Err != nil {
		return nil
	}
	// security fails for an item it has not; that is what was asked.
	_ = c.Run()
	return nil
}

// keyringEnvironment is what secret-tool needs beyond a command's usual
// environment to reach the person's keyring.
func keyringEnvironment() []string {
	out := []string{}
	for _, key := range []string{"DBUS_SESSION_BUS_ADDRESS", "DISPLAY", "WAYLAND_DISPLAY"} {
		if v, ok := os.LookupEnv(key); ok {
			out = append(out, key+"="+v)
		}
	}
	return out
}
