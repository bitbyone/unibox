package unibox

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CRLFFiles are the env's files and its local kits' text files that have
// Windows line endings. Git on Windows checks a repository out so unless
// it says otherwise (core.autocrlf), and a kit built from such files fails
// in Linux: a heredoc in a RUN never ends, a script's #!/bin/sh\r is no
// shell. sbx builds the same files on macOS, where git leaves them be,
// which is why the env works there.
func CRLFFiles(env Env) []string {
	out := []string{}
	check := func(path string) {
		b, err := os.ReadFile(path)
		if err != nil || len(b) > 1<<20 {
			return
		}
		head := b[:min(len(b), 8000)]
		if bytes.IndexByte(head, 0) >= 0 {
			return
		}
		if bytes.Contains(b, []byte("\r\n")) {
			out = append(out, path)
		}
	}
	for _, f := range env.SBX.Files {
		check(f)
	}
	for _, k := range env.SBX.Kits {
		if k.Dir == "" {
			continue
		}
		_ = filepath.WalkDir(k.Dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type().IsRegular() {
				check(path)
			}
			return nil
		})
	}
	return out
}

// CheckLineEndings refuses an env whose files sbx would build into a kit
// that fails, before the minutes the build takes.
func CheckLineEndings(env Env) error {
	files := CRLFFiles(env)
	if len(files) == 0 {
		return nil
	}
	shown := files
	if len(shown) > 3 {
		shown = append(shown[:3:3], fmt.Sprintf("%d more", len(files)-3))
	}
	return Fail(4, "crlf", "env %s has Windows line endings in %s, which fail in the sandbox's Linux; %s", env.Name, strings.Join(shown, ", "), crlfHint(env))
}

func crlfHint(env Env) string {
	dir := env.Config.Repo
	if dir == "" {
		return "check its files out with LF line endings (git config core.autocrlf false, then check them out again)"
	}
	return fmt.Sprintf("with nothing uncommitted in %s, run there git config core.autocrlf false, git rm --cached -r -q . and git reset --hard; a .gitattributes of * text=auto eol=lf in the team's repository spares everyone this", dir)
}
