//go:build cgo

package unibox

import (
	"context"
	"fmt"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

// msbBuiltIn says unibox brings microsandbox along: the SDK can install the
// runtime it was released with, msb and libkrunfw, into ~/.microsandbox.
const msbBuiltIn = true

// msbResolve finds an installed runtime without installing one.
func msbResolve() (string, error) {
	rt, err := microsandbox.ResolveRuntime(microsandbox.RuntimeConfig{})
	if err != nil {
		return "", err
	}
	return rt.MSBPath, nil
}

// msbInstall installs the runtime the SDK was released with, or finds the
// one already there.
func msbInstall(ctx context.Context) (string, error) {
	rt, err := microsandbox.EnsureRuntime(ctx, microsandbox.RuntimeConfig{}, microsandbox.InstallOptions{})
	if err != nil {
		return "", fmt.Errorf("could not install the microsandbox runtime; install it with curl -fsSL https://install.microsandbox.dev | sh: %w", err)
	}
	return rt.MSBPath, nil
}

// msbRotate gives a running sandbox's secrets the values now in the
// variables they name, keeping their placeholders, so the sandbox's
// processes go on with the same ones. msb's CLI resets a placeholder it
// rotates, which needs a restart; the SDK does not.
func msbRotate(ctx context.Context, sandbox string, secrets []msbSecret) error {
	h, err := microsandbox.GetSandbox(ctx, sandbox)
	if err != nil {
		return err
	}
	specs := map[string]microsandbox.SecretModifySpec{}
	for _, s := range secrets {
		specs[s.Env] = microsandbox.SecretModifySpec{Env: s.Source, Placeholder: s.Placeholder}
	}
	plan, err := h.Modify(ctx, microsandbox.ModifyOptions{Secrets: specs, Policy: microsandbox.ModificationPolicyNoRestart})
	if err != nil {
		return err
	}
	if !plan.Applied {
		return fmt.Errorf("msb did not rotate the secrets of sandbox %s live", sandbox)
	}
	return nil
}
