package unibox

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// buildImage builds a profile's image with the host's Docker, checks that
// every declared agent is in it, and hands the image to the runtime as a
// Docker archive through load: both runtimes take one, so the build is the
// same for each and only the loading differs.
func buildImage(ctx context.Context, cache, runtime string, p Profile, tag string, log io.Writer, load func(archive string) error) (string, error) {
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(cache, "build-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if p.Image != "" {
		if err = runLogged(hostCommand(ctx, "docker", "pull", p.Image), log); err != nil {
			return "", err
		}
		if err = runLogged(hostCommand(ctx, "docker", "tag", p.Image, tag), log); err != nil {
			return "", err
		}
	} else {
		for name, file := range p.BuildFiles() {
			dest := filepath.Join(dir, filepath.FromSlash(name))
			if err = AtomicWrite(dest, file.Data, os.FileMode(file.Mode)); err != nil {
				return "", err
			}
		}
		install, err := p.InstallContext()
		if err != nil {
			return "", err
		}
		installDir, err := os.MkdirTemp(cache, "install-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(installDir)
		if err = writeInstallContext(installDir, install); err != nil {
			return "", err
		}
		args := []string{"build", "--tag", tag, "--file", filepath.Join(dir, p.Dockerfile), "--build-context", "unibox=" + installDir}
		buildArgs, err := p.BuildArgs(runtime)
		if err != nil {
			return "", err
		}
		for _, arg := range buildArgs {
			args = append(args, "--build-arg", arg)
		}
		args = append(args, dir)
		if err = runLogged(hostCommand(ctx, "docker", args...), log); err != nil {
			return "", err
		}
	}
	for _, id := range p.Agents {
		a, _ := FindAgent(id)
		fmt.Fprintf(log, "checking agent %s\n", id)
		cmd := hostCommand(ctx, "docker", "run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--entrypoint", "/bin/sh", tag, "-c", "command -v "+Quote(a.Command))
		if err = runLogged(cmd, log); err != nil {
			return "", fmt.Errorf("profile %s declares %s but its image cannot find it; install it in the Dockerfile", p.Name, id)
		}
	}
	archive := filepath.Join(dir, "image.tar")
	if err = runLogged(hostCommand(ctx, "docker", "image", "save", "--output", archive, tag), log); err != nil {
		return "", err
	}
	if err = load(archive); err != nil {
		return "", err
	}
	b, err := commandOutput(hostCommand(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", tag))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
