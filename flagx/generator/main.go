package main

import (
	"fmt"
	"go/format"
	"os"
)

type generatedOutput struct {
	path string
	data []byte
}

func main() {
	spec := newGenerationSpec()
	outputs := generatedOutputs(spec)

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

func generatedOutputs(spec generationSpec) []generatedOutput {
	builders := []struct {
		path  string
		build func(*generator, generationSpec) []byte
	}{
		{path: constraintsFile, build: (*generator).constraintsFile},
		{path: codecFile, build: (*generator).codecsFile},
		{path: flagFile, build: (*generator).flagsFile},
	}

	outputs := make([]generatedOutput, 0, len(builders))
	for _, builder := range builders {
		g := &generator{}
		outputs = append(outputs, generatedOutput{
			path: builder.path,
			data: builder.build(g, spec),
		})
	}

	return outputs
}
