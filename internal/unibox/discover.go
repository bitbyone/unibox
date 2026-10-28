package unibox

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// SBXEnvSuffix ends the name of every file sbx reads as an env.
const SBXEnvSuffix = ".sbxenv.yaml"

// findLimit bounds how much of a directory init looks through: run outside
// a repository, in a home directory, it would otherwise read all of it.
const findLimit = 50000

var errTooMany = errors.New("too many files")

// FindSBXEnvs lists the sbxenv files under root, sorted. Hidden directories
// and node_modules are passed over: a team keeps its envs in neither, and
// they are where most files of a checkout are.
func FindSBXEnvs(root string) ([]string, error) {
	found, seen := []string{}, 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A directory that cannot be read holds nothing init could use.
			if d != nil && d.IsDir() && path != root {
				return fs.SkipDir
			}
			return err
		}
		if seen++; seen > findLimit {
			return errTooMany
		}
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), SBXEnvSuffix) {
			found = append(found, path)
		}
		return nil
	})
	if errors.Is(err, errTooMany) {
		return nil, fmt.Errorf("%s holds more than %d files; run unibox init in your team's repository, or name its directory", root, findLimit)
	}
	slices.Sort(found)
	return found, err
}

// SBXEnvsIn lists the sbxenv files directly in dir, not below it.
func SBXEnvsIn(dir string) ([]string, error) {
	found, err := filepath.Glob(filepath.Join(dir, "*"+SBXEnvSuffix))
	slices.Sort(found)
	return found, err
}

// RepoRoot is the top of the git repository dir is in, or "" outside one.
func RepoRoot(ctx context.Context, dir string) string {
	c := hostCommand(ctx, "git", append(gitGuard(ctx, dir), "-C", dir, "rev-parse", "--show-toplevel")...)
	b, err := c.Output()
	if err != nil {
		return ""
	}
	root := strings.TrimSpace(string(b))
	// git resolves symbolic links; the user's own spelling of the path is
	// kept where it is the same directory.
	if same, err := sameDir(dir, root); err == nil && same {
		return dir
	}
	return root
}

func sameDir(a, b string) (bool, error) {
	ia, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ia, ib), nil
}

// EnvNameFor is the name an env made of file gets unless told: the name the
// file gives its sandbox, else the file's own, lower case.
func EnvNameFor(file string) string {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(file), SBXEnvSuffix))
	if e, err := ReadSBXEnv([]string{file}); err == nil {
		name = cmp.Or(e.Name, name)
	}
	return name
}

// EnvOf is the env whose first file is file, the one it was made from.
func (c Config) EnvOf(file string) (string, bool) {
	for _, name := range sortedKeys(c.Envs) {
		if files := c.Envs[name].Files; len(files) > 0 && files[0] == file {
			return name, true
		}
	}
	return "", false
}
