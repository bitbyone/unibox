//go:build !cgo

package unibox

import (
	"context"
	"fmt"
	"os/exec"
)

// Without cgo there is no SDK: unibox uses an msb the user installed.
const msbBuiltIn = false

func msbResolve() (string, error) { return exec.LookPath("msb") }

func msbInstall(ctx context.Context) (string, error) {
	path, err := msbResolve()
	if err != nil {
		return "", fmt.Errorf("this unibox was built without cgo and cannot install microsandbox; install it with curl -fsSL https://install.microsandbox.dev | sh")
	}
	return path, nil
}

func msbRotate(ctx context.Context, sandbox string, secrets []msbSecret) error {
	return fmt.Errorf("this unibox was built without cgo and cannot rotate secrets live; restart sandbox %s with unibox stop, then run it again", sandbox)
}
