package main

import (
	"os"

	"codecli-channels/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:], os.Stdout, os.Stderr))
}

func resolveConfigPath(path string) string {
	return app.ResolveConfigPath(path)
}
