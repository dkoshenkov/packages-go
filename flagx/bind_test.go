package flagx

import (
	"errors"
	"testing"

	"github.com/spf13/pflag"
)

func TestValueSetReturnsConstErrorForNilTarget(t *testing.T) {
	err := (*value[string])(nil).Set("test")
	if !errors.Is(err, errNilTarget) {
		t.Fatalf("Set error = %v, want %v", err, errNilTarget)
	}
}

func TestValueSetReturnsConstErrorForNilParser(t *testing.T) {
	var target string

	err := (&value[string]{target: &target}).Set("test")
	if !errors.Is(err, errNilParser) {
		t.Fatalf("Set error = %v, want %v", err, errNilParser)
	}
}

func TestAnyPanicsForNilTarget(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)

	defer func() {
		recovered := recover()
		if recovered != errNilTarget {
			t.Fatalf("panic = %v, want %v", recovered, errNilTarget)
		}
	}()

	Any(flags, "target", (*string)(nil), "Target", Codec[string]{
		Parse:  func(value string) (string, error) { return value, nil },
		Format: func(value string) string { return value },
		Type:   "string",
	})
}
