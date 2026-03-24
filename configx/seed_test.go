package configx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestLoadSeedsYAMLAndENVFromFlags(t *testing.T) {
	type cfg struct {
		Port  int    `cfgx:"port,default=8080"`
		Token string `cfgx:"token"`
	}

	yamlPath := filepath.Join(t.TempDir(), "config.seed.yaml")
	envPath := filepath.Join(t.TempDir(), ".env.seed")

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)

	var got cfg
	err := Load(context.Background(), &got,
		WithProfile("dev"),
		WithFlagSet(flags),
		ParseFlags(
			"--cfgx-seed-defaults",
			"--cfgx-seed-targets=yaml,env",
			"--cfgx-seed-yaml-file="+yamlPath,
			"--cfgx-seed-env-file="+envPath,
			"--cfgx-seed-only",
		),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	yamlContent, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	yamlText := string(yamlContent)
	if !strings.Contains(yamlText, "dev:") ||
		!strings.Contains(yamlText, "port: 8080") ||
		!strings.Contains(yamlText, "token: null") {
		t.Fatalf("unexpected yaml:\n%s", yamlText)
	}

	envContent, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	envText := string(envContent)
	if !strings.Contains(envText, "DEV_PORT=8080") ||
		!strings.Contains(envText, "DEV_TOKEN=") {
		t.Fatalf("unexpected env:\n%s", envText)
	}
}

func TestLoadSeedForceOverridesENVFile(t *testing.T) {
	type cfg struct {
		Port int `cfgx:"port,default=8080"`
	}

	envPath := filepath.Join(t.TempDir(), ".env.seed")
	if err := os.WriteFile(envPath, []byte("DEV_PORT=9999\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}

	t.Run("without force", func(t *testing.T) {
		flags := pflag.NewFlagSet("test-no-force", pflag.ContinueOnError)
		var got cfg
		err := Load(context.Background(), &got,
			WithProfile("dev"),
			WithFlagSet(flags),
			ParseFlags(
				"--cfgx-seed-defaults",
				"--cfgx-seed-targets=env",
				"--cfgx-seed-env-file="+envPath,
				"--cfgx-seed-only",
			),
		)
		if err != nil {
			t.Fatalf("load: %v", err)
		}

		content, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatalf("read env: %v", err)
		}
		if !strings.Contains(string(content), "DEV_PORT=9999") {
			t.Fatalf("unexpected env:\n%s", string(content))
		}
	})

	t.Run("with force", func(t *testing.T) {
		flags := pflag.NewFlagSet("test-force", pflag.ContinueOnError)
		var got cfg
		err := Load(context.Background(), &got,
			WithProfile("dev"),
			WithFlagSet(flags),
			ParseFlags(
				"--cfgx-seed-defaults",
				"--cfgx-seed-targets=env",
				"--cfgx-seed-env-file="+envPath,
				"--cfgx-seed-force",
				"--cfgx-seed-only",
			),
		)
		if err != nil {
			t.Fatalf("load: %v", err)
		}

		content, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatalf("read env: %v", err)
		}
		if !strings.Contains(string(content), "DEV_PORT=8080") {
			t.Fatalf("unexpected env:\n%s", string(content))
		}
	})
}

func TestLoadReturnsErrorForInvalidSeedTarget(t *testing.T) {
	type cfg struct {
		Port int `cfgx:"port,default=8080"`
	}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var got cfg
	err := Load(context.Background(), &got,
		WithProfile("dev"),
		WithFlagSet(flags),
		ParseFlags("--cfgx-seed-targets=unknown"),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), errSeedInvalidTarget.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWithVaultCredentialsValidation(t *testing.T) {
	type cfg struct {
		Port int `cfgx:"port,optional"`
	}

	var got cfg
	err := Load(context.Background(), &got,
		WithProfile("dev"),
		WithVaultCredentials(VaultCredentials{
			Path: "secret/data/app",
		}),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), errVaultCredentialsMissingAddr.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}
