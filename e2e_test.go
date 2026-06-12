package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBinarySmoke(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "mt5-pnl-cli")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "mt5-pnl-cli dev (schema 1.0)") {
		t.Errorf("version output: %q", out)
	}

	// pnl with no --snapshot and no env var fails before touching the keychain.
	cmd := exec.Command(bin, "pnl")
	cmd.Env = envWithout("MT5_PNL_SNAPSHOT")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("want exit 1, got %v", err)
	}
	if !strings.Contains(stderr.String(), "MT5_PNL_SNAPSHOT") {
		t.Errorf("stderr: %q", stderr.String())
	}

	// set-passphrase refuses to run without an interactive terminal.
	cmd = exec.Command(bin, "set-passphrase")
	cmd.Stdin = strings.NewReader("")
	stderr.Reset()
	cmd.Stderr = &stderr
	err = cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("want exit 1, got %v", err)
	}
	if !strings.Contains(stderr.String(), "interactive terminal") {
		t.Errorf("stderr: %q", stderr.String())
	}
}

func envWithout(name string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, name+"=") {
			env = append(env, kv)
		}
	}
	return env
}
