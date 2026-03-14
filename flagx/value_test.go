package flagx

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

type generationTarget string

const (
	generateAll generationTarget = "all"
	generateGo  generationTarget = "go"
	generateAPI generationTarget = "oapi"
)

type retryCount int

type endpoint struct {
	host string
	port int
}

func TestStringPWorksWithStringAliases(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	target := generateAll

	StringP(flags, "target", "t", &target, "Artifacts to generate", OneOf(generateAll, generateGo, generateAPI))

	if err := flags.Set("target", "go"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	if target != generateGo {
		t.Fatalf("target = %q, want %q", target, generateGo)
	}
}

func TestStringPParseWorksWithStringAliases(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	target := generateAll

	StringP(flags, "target", "t", &target, "Artifacts to generate", OneOf(generateAll, generateGo, generateAPI))

	if err := flags.Parse([]string{"--target", "go"}); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if target != generateGo {
		t.Fatalf("target = %q, want %q", target, generateGo)
	}
}

func TestStringPRejectsUnexpectedValue(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	target := generateAll

	StringP(flags, "target", "t", &target, "Artifacts to generate", OneOf(generateAll, generateGo, generateAPI))

	err := flags.Set("target", "other")
	if err == nil {
		t.Fatal("Set returned nil error for invalid enum value")
	}

	if !strings.Contains(err.Error(), "must be one of all, go, oapi") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIntPUsesDefaultOption(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var retries retryCount

	IntP(flags, "retries", "r", &retries, "Number of retries", Default(retryCount(3)))

	if retries != 3 {
		t.Fatalf("retries = %d, want %d", retries, 3)
	}
}

func TestBoolParseWithoutArgumentSetsTrue(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var verbose bool

	Bool(flags, "verbose", &verbose, "Enable verbose logging")

	if err := flags.Parse([]string{"--verbose"}); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !verbose {
		t.Fatal("verbose = false, want true")
	}
}

func TestBoolPShorthandParseWithoutArgumentSetsTrue(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var verbose bool

	BoolP(flags, "verbose", "v", &verbose, "Enable verbose logging")

	if err := flags.Parse([]string{"-v"}); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !verbose {
		t.Fatal("verbose = false, want true")
	}
}

func TestAnyUsesCustomBoolNoOptDefVal(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	type toggle bool

	var feature toggle
	Any(flags, "feature", &feature, "Enable feature", Codec[toggle]{
		Parse: func(value string) (toggle, error) {
			switch value {
			case "enabled":
				return true, nil
			case "disabled":
				return false, nil
			default:
				return false, errors.New("unexpected")
			}
		},
		Format: func(value toggle) string {
			if value {
				return "enabled"
			}

			return "disabled"
		},
		Type:        "toggle",
		IsBool:      true,
		NoOptDefVal: "enabled",
	})

	if err := flags.Parse([]string{"--feature"}); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !feature {
		t.Fatal("feature = false, want true")
	}
}

func TestValidateNilIsIgnored(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var target string

	String(flags, "target", &target, "Artifacts to generate", Validate[string](nil))

	if err := flags.Set("target", "go"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	if target != "go" {
		t.Fatalf("target = %q, want %q", target, "go")
	}
}

func TestStringWithInvalidDefaultPanicsAtBind(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var target generationTarget

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("bind did not panic for invalid default value")
		}

		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("panic = %T, want error", recovered)
		}

		if !strings.Contains(err.Error(), "invalid initial value") {
			t.Fatalf("panic = %v, want invalid initial value error", err)
		}

		if !strings.Contains(err.Error(), "must be one of all, go, oapi") {
			t.Fatalf("panic = %v, want validator message", err)
		}
	}()

	String(flags, "target", &target, "Artifacts to generate", Default(generationTarget("other")), OneOf(generateAll, generateGo, generateAPI))
}

func TestStringWithInvalidInitialValuePanicsAtBind(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	target := generationTarget("other")

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("bind did not panic for invalid initial value")
		}

		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("panic = %T, want error", recovered)
		}

		if !strings.Contains(err.Error(), "invalid initial value") {
			t.Fatalf("panic = %v, want invalid initial value error", err)
		}

		if !strings.Contains(err.Error(), "must be one of all, go, oapi") {
			t.Fatalf("panic = %v, want validator message", err)
		}
	}()

	String(flags, "target", &target, "Artifacts to generate", OneOf(generateAll, generateGo, generateAPI))
}

func TestAnyPParsesCustomType(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var addr endpoint

	AnyP(flags, "endpoint", "e", &addr, "Bind endpoint", Codec[endpoint]{
		Parse: func(value string) (endpoint, error) {
			host, port, ok := strings.Cut(value, ":")
			if !ok {
				return endpoint{}, strconv.ErrSyntax
			}

			number, err := strconv.Atoi(port)
			if err != nil {
				return endpoint{}, err
			}

			return endpoint{host: host, port: number}, nil
		},
		Format: func(value endpoint) string {
			return value.host + ":" + strconv.Itoa(value.port)
		},
		Type: "endpoint",
	})

	if err := flags.Set("endpoint", "localhost:8080"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	if addr.host != "localhost" || addr.port != 8080 {
		t.Fatalf("addr = %#v, want localhost:8080", addr)
	}
}

func TestAnyPParseRejectsUnexpectedValue(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	target := generateAll

	AnyP(flags, "target", "t", &target, "Artifacts to generate", Codec[generationTarget]{
		Parse:  func(value string) (generationTarget, error) { return generationTarget(value), nil },
		Format: func(value generationTarget) string { return string(value) },
		Type:   "string",
	}, OneOf(generateAll, generateGo, generateAPI))

	err := flags.Parse([]string{"-t", "other"})
	if err == nil {
		t.Fatal("Parse returned nil error for invalid enum value")
	}

	if !strings.Contains(err.Error(), "must be one of all, go, oapi") {
		t.Fatalf("unexpected error: %v", err)
	}
}
