package configx

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestBindGlobalFlagsAndLoad(t *testing.T) {
	type serverCfg struct {
		I int `cfgx:"i"`
	}
	type cfg struct {
		Port   int       `cfgx:"port"`
		Debug  bool      `cfgx:"debug"`
		Server serverCfg `cfgx:"server"`
	}

	original := pflag.CommandLine
	defer func() {
		pflag.CommandLine = original
	}()

	pflag.CommandLine = pflag.NewFlagSet("app", pflag.ContinueOnError)

	if err := BindGlobalFlags(&cfg{}); err != nil {
		t.Fatalf("bind global flags: %v", err)
	}

	if err := pflag.CommandLine.Parse([]string{"--port=9000", "--debug", "--server-i=7"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	var got cfg
	if err := Load(context.Background(), &got, WithProfile("dev"), WithFlagSet(pflag.CommandLine)); err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.Port != 9000 {
		t.Fatalf("port = %d, want 9000", got.Port)
	}
	if !got.Debug {
		t.Fatal("debug = false, want true")
	}
	if got.Server.I != 7 {
		t.Fatalf("server.i = %d, want 7", got.Server.I)
	}
}

func TestBindFlagsRejectsDuplicate(t *testing.T) {
	type cfg struct {
		Port int `cfgx:"port"`
	}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if err := BindFlags(flags, &cfg{}); err != nil {
		t.Fatalf("first bind: %v", err)
	}

	err := BindFlags(flags, &cfg{})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBindFlagsUsesDefaultValues(t *testing.T) {
	type cfg struct {
		Port    int  `cfgx:"port,default=8080"`
		Enabled bool `cfgx:"enabled,default=true"`
	}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if err := BindFlags(flags, &cfg{}); err != nil {
		t.Fatalf("bind flags: %v", err)
	}

	port := flags.Lookup("port")
	if port == nil {
		t.Fatal("port flag not found")
	}
	if port.DefValue != "8080" {
		t.Fatalf("port def = %q, want 8080", port.DefValue)
	}

	enabled := flags.Lookup("enabled")
	if enabled == nil {
		t.Fatal("enabled flag not found")
	}
	if enabled.DefValue != "true" {
		t.Fatalf("enabled def = %q, want true", enabled.DefValue)
	}
}

func TestLoadWithParseFlagsOption(t *testing.T) {
	type cfg struct {
		Port  int  `cfgx:"port"`
		Debug bool `cfgx:"debug,optional"`
	}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)

	var got cfg
	err := Load(context.Background(), &got,
		WithProfile("dev"),
		WithFlagSet(flags),
		ParseFlags("--port=9000", "--debug"),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.Port != 9000 {
		t.Fatalf("port = %d, want 9000", got.Port)
	}
	if !got.Debug {
		t.Fatal("debug = false, want true")
	}
}

func TestLoadWithParseFlagsFailsWhenAlreadyParsed(t *testing.T) {
	type cfg struct {
		Port int `cfgx:"port"`
	}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if err := flags.Parse(nil); err != nil {
		t.Fatalf("parse empty: %v", err)
	}

	var got cfg
	err := Load(context.Background(), &got,
		WithProfile("dev"),
		WithFlagSet(flags),
		ParseFlags("--port=9000"),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), errFlagSetAlreadyParsed.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadWithParseFlagsSupportsVaultCredentialsFlags(t *testing.T) {
	type cfg struct{}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var got cfg
	err := Load(context.Background(), &got,
		WithProfile("dev"),
		WithFlagSet(flags),
		ParseFlags(
			"--cfgx-vault-address=http://127.0.0.1:8200",
			"--cfgx-vault-path=secret/data/myapp",
			"--cfgx-vault-token=token-from-flag",
		),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
}
