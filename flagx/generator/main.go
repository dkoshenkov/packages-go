package main

import (
	"fmt"
	"go/format"
	"os"
)

func main() {
	spec := newGenerationSpec()
	g := &generator{}
	outputs := []struct {
		path string
		data []byte
	}{
		{path: constraintsFile, data: g.constraintsFile(spec)},
		{path: codecFile, data: g.codecsFile(spec)},
		{path: flagFile, data: g.flagsFile(spec)},
	}

	for _, output := range outputs {
		formatted, err := format.Source(output.data)
		if err != nil {
			fail(fmt.Sprintf("format %s", output.path), err)
		}

		if err := os.WriteFile(output.path, formatted, 0o644); err != nil {
			fail(fmt.Sprintf("write %s", output.path), err)
		}
	}
}
