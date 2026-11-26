package main

import (
	"embed"
	"log"
	"os"

	"github.com/gongjs/agym/internal/cli"
	"github.com/gongjs/agym/internal/gui"
)

//go:embed frontend
var frontend embed.FS

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "ui" {
		if err := gui.Run(frontend); err != nil {
			log.Fatal(err)
		}
		return
	}
	os.Exit(cli.Run(args, os.Stdout, os.Stderr))
}
