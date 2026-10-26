package unibox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

var secretName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidSecretName says whether a name is one sbx stores a service's token
// under, as a kit's credentials and an env's bindings name them.
func ValidSecretName(name string) bool { return secretName.MatchString(name) }

// NeededSecret is a service an env's sandbox takes a token for, and
// whether sbx has one stored: sbx's secrets are set once and used by every
// sandbox that needs them.
type NeededSecret struct {
	Service     string `json:"service"`
	Description string `json:"description,omitempty"`
	// Optional is a credential a kit can do without.
	Optional bool `json:"optional,omitempty"`
	// Kits are the kits asking for it; empty for one only the env's
	// bindings name.
	Kits   []string `json:"kits,omitempty"`
	Stored bool     `json:"stored"`
	// Scope is where the token is stored: global, or a sandbox's name.
	Scope string `json:"scope,omitempty"`
}

// NeededSecrets are the services an env needs tokens for: those its kits
// declare and those its bindings name, each with whether sbx has one,
// globally or for the sandbox alone. sandbox "" is the one the env
// creates, which a secret may already be stored for.
func (m *Manager) NeededSecrets(ctx context.Context, env Env, sandbox string) ([]NeededSecret, error) {
	byService := map[string]*NeededSecret{}
	order := []string{}
	add := func(service string) *NeededSecret {
		if l, ok := byService[service]; ok {
			return l
		}
		l := &NeededSecret{Service: service, Optional: true}
		byService[service] = l
		order = append(order, service)
		return l
	}
	if sandbox == "" {
		sandbox = env.SandboxName()
	}
	// sbx's secrets are listed while the kits are read.
	var secrets []Secret
	var secretsErr error
	listed := make(chan struct{})
	go func() {
		defer close(listed)
		secrets, secretsErr = m.SBX.Secrets(ctx)
	}()
	kits, err := m.readKits(ctx, env)
	<-listed
	if err != nil {
		return nil, err
	}
	for i, k := range env.SBX.Kits {
		for _, c := range kits[i].Credentials {
			l := add(c.Service)
			if l.Description == "" {
				l.Description = c.Description
			}
			l.Optional = l.Optional && c.Optional
			l.Kits = append(l.Kits, kitName(k.Source))
		}
	}
	for _, service := range env.SBX.Bindings {
		add(service).Optional = false
	}
	if secretsErr != nil {
		return nil, fmt.Errorf("cannot list sbx's secrets: %w", secretsErr)
	}
	out := []NeededSecret{}
	for _, service := range order {
		l := *byService[service]
		for _, s := range secrets {
			if s.Name != service {
				continue
			}
			if s.Scope == "global" || sandbox != "" && s.Scope == sandbox {
				l.Stored, l.Scope = true, s.Scope
			}
		}
		out = append(out, l)
	}
	slices.SortStableFunc(out, func(a, b NeededSecret) int {
		switch {
		case !a.Optional && b.Optional:
			return -1
		case a.Optional && !b.Optional:
			return 1
		}
		return 0
	})
	return out, nil
}

// readKits reads an env's kits as sbx sees them, each with the args the
// env gives it, which may be the person's. sbx reads them one at a time,
// a kit of a registry for seconds, so what it said is kept: in memory for
// a list drawn again, and on disk for the next command - for a kit of
// files until one of them changes, for a registry's until a day passes or,
// pinned by digest, for good.
func (m *Manager) readKits(ctx context.Context, env Env) ([]Kit, error) {
	values, _ := m.Paths.LoadEnvValues()
	kits := make([]Kit, len(env.SBX.Kits))
	errs := make([]error, len(env.SBX.Kits))
	var wg sync.WaitGroup
	for i, k := range env.SBX.Kits {
		ref := k.Source
		if k.Dir != "" {
			ref = k.Dir
		}
		args := env.SBX.KitArgs(k, values.ArgsFor(env))
		key := ref
		for _, name := range sortedKeys(args) {
			key += "\x00" + name + "=" + args[name]
		}
		if kit, ok := m.kits.Load(key); ok {
			kits[i] = kit.(Kit)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			file, fresh := m.kitCacheFile(key, k.Dir)
			if file != "" {
				if b, err := os.ReadFile(file); err == nil && fresh(file) && json.Unmarshal(b, &kits[i]) == nil {
					m.kits.Store(key, kits[i])
					return
				}
			}
			if kits[i], errs[i] = m.SBX.KitInspect(ctx, ref, args); errs[i] != nil {
				return
			}
			m.kits.Store(key, kits[i])
			if b, err := json.Marshal(kits[i]); err == nil && file != "" && os.MkdirAll(filepath.Dir(file), 0o700) == nil {
				_ = os.WriteFile(file, b, 0o600)
			}
		}()
	}
	wg.Wait()
	return kits, errors.Join(errs...)
}

// kitCacheFile is where what sbx said of a kit is kept, and whether what
// is kept there still holds. A kit of files is named by the files' names,
// sizes and times, so a change to one is a file not yet written.
func (m *Manager) kitCacheFile(key, dir string) (string, func(string) bool) {
	always := func(string) bool { return true }
	h := sha256.New()
	h.Write([]byte(key))
	fresh := always
	switch {
	case dir != "":
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "\x00%s %d %d", path, info.Size(), info.ModTime().UnixNano())
			return nil
		})
		if err != nil {
			return "", always
		}
	case !strings.Contains(key, "@sha256:"):
		fresh = func(file string) bool {
			info, err := os.Stat(file)
			return err == nil && time.Since(info.ModTime()) < 24*time.Hour
		}
	}
	return filepath.Join(m.Paths.Cache, "kits", hex.EncodeToString(h.Sum(nil))+".json"), fresh
}
