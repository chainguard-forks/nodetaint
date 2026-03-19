package config

import (
	"os"
	"testing"

	"github.com/jessevdk/go-flags"
)

func TestOps_Defaults(t *testing.T) {
	opts := &Ops{}
	parser := flags.NewParser(opts, flags.Default&^flags.PrintErrors)
	_, err := parser.ParseArgs([]string{})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if opts.LogLevel != "info" {
		t.Errorf("expected default LogLevel 'info', got %q", opts.LogLevel)
	}
	if opts.BindAddr != ":9797" {
		t.Errorf("expected default BindAddr ':9797', got %q", opts.BindAddr)
	}
	if opts.NodeTaint != "" {
		t.Errorf("expected empty NodeTaint by default, got %q", opts.NodeTaint)
	}
	if opts.DaemonSetAnnotation != "" {
		t.Errorf("expected empty DaemonSetAnnotation by default, got %q", opts.DaemonSetAnnotation)
	}
}

func TestOps_CommandLineFlags(t *testing.T) {
	opts := &Ops{}
	parser := flags.NewParser(opts, flags.Default&^flags.PrintErrors)
	_, err := parser.ParseArgs([]string{
		"--log-level", "debug",
		"--node-taint", "custom-taint",
		"--daemonset-annotation", "my-annotation",
		"--bind-address", ":8080",
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if opts.LogLevel != "debug" {
		t.Errorf("expected LogLevel 'debug', got %q", opts.LogLevel)
	}
	if opts.NodeTaint != "custom-taint" {
		t.Errorf("expected NodeTaint 'custom-taint', got %q", opts.NodeTaint)
	}
	if opts.DaemonSetAnnotation != "my-annotation" {
		t.Errorf("expected DaemonSetAnnotation 'my-annotation', got %q", opts.DaemonSetAnnotation)
	}
	if opts.BindAddr != ":8080" {
		t.Errorf("expected BindAddr ':8080', got %q", opts.BindAddr)
	}
}

func TestOps_ShortFlag(t *testing.T) {
	opts := &Ops{}
	parser := flags.NewParser(opts, flags.Default&^flags.PrintErrors)
	_, err := parser.ParseArgs([]string{"-p", ":3000"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if opts.BindAddr != ":3000" {
		t.Errorf("expected BindAddr ':3000' via short flag, got %q", opts.BindAddr)
	}
}

func TestOps_EnvironmentVariables(t *testing.T) {
	os.Setenv("LOG_LEVEL", "warn")
	os.Setenv("NODE_TAINT", "env-taint")
	os.Setenv("DAEMONSET_ANNOTATION", "env-annotation")
	os.Setenv("BIND_ADDRESS", ":9999")
	defer func() {
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("NODE_TAINT")
		os.Unsetenv("DAEMONSET_ANNOTATION")
		os.Unsetenv("BIND_ADDRESS")
	}()

	opts := &Ops{}
	parser := flags.NewParser(opts, flags.Default&^flags.PrintErrors)
	_, err := parser.ParseArgs([]string{})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if opts.LogLevel != "warn" {
		t.Errorf("expected LogLevel 'warn' from env, got %q", opts.LogLevel)
	}
	if opts.NodeTaint != "env-taint" {
		t.Errorf("expected NodeTaint 'env-taint' from env, got %q", opts.NodeTaint)
	}
	if opts.DaemonSetAnnotation != "env-annotation" {
		t.Errorf("expected DaemonSetAnnotation 'env-annotation' from env, got %q", opts.DaemonSetAnnotation)
	}
	if opts.BindAddr != ":9999" {
		t.Errorf("expected BindAddr ':9999' from env, got %q", opts.BindAddr)
	}
}

func TestOps_FlagOverridesEnv(t *testing.T) {
	os.Setenv("LOG_LEVEL", "warn")
	defer os.Unsetenv("LOG_LEVEL")

	opts := &Ops{}
	parser := flags.NewParser(opts, flags.Default&^flags.PrintErrors)
	_, err := parser.ParseArgs([]string{"--log-level", "error"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if opts.LogLevel != "error" {
		t.Errorf("expected flag to override env: want 'error', got %q", opts.LogLevel)
	}
}
