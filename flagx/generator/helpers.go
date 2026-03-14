package main

import (
	"fmt"
	"os"
	"strings"
)

func list(items ...string) []string {
	return items
}

func joinWithPipe(items []string) string {
	return joinWith(items, " | ")
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "flagxgen: %s: %v\n", step, err)
	os.Exit(1)
}

func joinComma(items []string) string {
	return joinWith(items, ", ")
}

func joinWith(items []string, sep string) string {
	return strings.Join(items, sep)
}
