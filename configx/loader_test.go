package configx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

type mapVault struct {
	values map[string]string
	err    error
}

func (v mapVault) Get(_ context.Context, key string) (string, bool, error) {
	if v.err != nil {
		return "", false, v.err
	}

	value, ok := v.values[key]
	return value, ok, nil
}

func TestLoadUsesSourcePriority(t *testing.T) {
	type cfg struct {
		Port int `cfgx:"port"`
	}

	yamlPath := writeYAMLFile(t, "port: 1\n")
	t.Setenv("PORT", "2")
	vault := mapVault{values: map[string]string{"PORT": "3"}}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Int("port", 0, "")
	if err := flags.Set("port", "4"); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	var got cfg
	err := Load(context.Background(), &got,
		WithProfile("dev"),
		WithYAMLFile(yamlPath),
		WithVault(vault),
		WithFlagSet(flags),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Port != 4 {
		t.Fatalf("port = %d, want 4", got.Port)
	}
}

func TestFlagSourceUsesOnlyChangedFlags(t *testing.T) {
	type cfg struct {
		Port int `cfgx:"port"`
	}

	t.Setenv("PORT", "12")

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Int("port", 777, "")

	var got cfg
	err := Load(context.Background(), &got,
		WithProfile("dev"),
		WithFlagSet(flags),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Port != 12 {
		t.Fatalf("port = %d, want 12", got.Port)
	}
}

func TestStrictGroupUsesOnlyProfileGroup(t *testing.T) {
	type cfg struct {
		A int `cfgx:"a"`
		B int `cfgx:"b"`
	}

	t.Setenv("PROD_A", "10")
	t.Setenv("B", "20")

	var got cfg
	err := Load(context.Background(), &got, WithProfile("prod"))
	if err == nil {
		t.Fatal("expected error")
	}
	if got.A != 10 {
		t.Fatalf("a = %d, want 10", got.A)
	}
	if !strings.Contains(err.Error(), "B: required value is missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOverlayDefaultPriority(t *testing.T) {
	type cfg struct {
		C int `cfgx:"c,default=7"`
	}

	t.Setenv("C", "9")

	t.Run("default over no-prefix", func(t *testing.T) {
		var got cfg
		err := Load(context.Background(), &got,
			WithProfile("dev"),
			WithResolveMode(OverlayDefaultHigh),
		)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got.C != 7 {
			t.Fatalf("c = %d, want 7", got.C)
		}
	})

	t.Run("no-prefix over default", func(t *testing.T) {
		var got cfg
		err := Load(context.Background(), &got,
			WithProfile("dev"),
			WithResolveMode(OverlayDefaultLow),
		)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got.C != 9 {
			t.Fatalf("c = %d, want 9", got.C)
		}
	})
}

func TestProfileAutoDetection(t *testing.T) {
	type cfg struct {
		Port int `cfgx:"port"`
	}

	t.Run("resolves from sources", func(t *testing.T) {
		t.Setenv("ENV", "prod")
		t.Setenv("PROD_PORT", "11")

		var got cfg
		err := Load(context.Background(), &got)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got.Port != 11 {
			t.Fatalf("port = %d, want 11", got.Port)
		}
	})

	t.Run("source order for env", func(t *testing.T) {
		yamlPath := writeYAMLFile(t, "env: dev\nprod:\n  port: 90\ndev:\n  port: 91\n")
		t.Setenv("ENV", "dev")

		vault := mapVault{values: map[string]string{"ENV": "dev"}}
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flags.String("env", "", "")
		if err := flags.Set("env", "prod"); err != nil {
			t.Fatalf("set flag env: %v", err)
		}

		var got cfg
		err := Load(context.Background(), &got,
			WithYAMLFile(yamlPath),
			WithVault(vault),
			WithFlagSet(flags),
		)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got.Port != 90 {
			t.Fatalf("port = %d, want 90", got.Port)
		}
	})

	t.Run("missing env returns error", func(t *testing.T) {
		var got cfg
		err := Load(context.Background(), &got)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "profile is not set") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid env returns error", func(t *testing.T) {
		t.Setenv("ENV", "stage")
		var got cfg
		err := Load(context.Background(), &got)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "invalid profile") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestNestedKeysAndOverrides(t *testing.T) {
	type server struct {
		I int    `cfgx:"i"`
		B string `cfgx:"b"`
	}
	type cfg struct {
		Server server `cfgx:"server"`
		Name   string `cfgx:"name" env:"CUSTOM_NAME" yaml:"custom.name"`
	}

	yamlPath := writeYAMLFile(t, ""+
		"dev:\n"+
		"  server:\n"+
		"    i: 1\n"+
		"    b: yaml\n"+
		"  custom:\n"+
		"    name: yaml_name\n",
	)
	t.Setenv("DEV_SERVER_I", "5")
	t.Setenv("DEV_SERVER_B", "env_b")
	t.Setenv("DEV_CUSTOM_NAME", "env_name")

	var got cfg
	err := Load(context.Background(), &got,
		WithProfile("dev"),
		WithYAMLFile(yamlPath),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.Server.I != 5 {
		t.Fatalf("server.i = %d, want 5", got.Server.I)
	}
	if got.Server.B != "env_b" {
		t.Fatalf("server.b = %q, want env_b", got.Server.B)
	}
	if got.Name != "env_name" {
		t.Fatalf("name = %q, want env_name", got.Name)
	}
}

func TestYAMLOverrideTag(t *testing.T) {
	type cfg struct {
		Name string `cfgx:"name" yaml:"custom.name"`
	}

	yamlPath := writeYAMLFile(t, ""+
		"dev:\n"+
		"  name: canonical\n"+
		"  custom:\n"+
		"    name: override\n",
	)

	var got cfg
	err := Load(context.Background(), &got,
		WithProfile("dev"),
		WithYAMLFile(yamlPath),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Name != "override" {
		t.Fatalf("name = %q, want override", got.Name)
	}
}

func TestRequiredOptionalAndAllowMissing(t *testing.T) {
	type cfg struct {
		Required int `cfgx:"required"`
		Optional int `cfgx:"optional,optional"`
	}

	t.Run("required by default", func(t *testing.T) {
		var got cfg
		err := Load(context.Background(), &got, WithProfile("dev"))
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "Required: required value is missing") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("allow missing globally", func(t *testing.T) {
		var got cfg
		err := Load(context.Background(), &got, WithProfile("dev"), WithAllowMissing())
		if err != nil {
			t.Fatalf("load: %v", err)
		}
	})
}

func TestAggregatesErrors(t *testing.T) {
	type cfg struct {
		A int    `cfgx:"a"`
		B func() `cfgx:"b"`
		C int    `cfgx:"c,default=not_an_int"`
	}

	var got cfg
	err := Load(context.Background(), &got, WithProfile("dev"))
	if err == nil {
		t.Fatal("expected error")
	}

	message := err.Error()
	if !strings.Contains(message, "A: required value is missing") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(message, "B: unsupported type") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(message, "C: decode default") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMustLoadPanics(t *testing.T) {
	type cfg struct {
		Port int `cfgx:"port"`
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
	}()

	var got cfg
	MustLoad(context.Background(), &got, WithProfile("dev"))
}

func TestMalformedTagReturnsError(t *testing.T) {
	type cfg struct {
		Port int `cfgx:"port,unknown=1"`
	}

	var got cfg
	err := Load(context.Background(), &got, WithProfile("dev"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown option") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeYAMLFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write yaml file: %v", err)
	}

	return path
}
