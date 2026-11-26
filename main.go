package main

import (
	"log"
	"os"

	"github.com/gongjs/agym/internal/cli"
	"github.com/gongjs/agym/internal/gui"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "ui" {
		if err := gui.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}
	os.Exit(cli.Run(args, os.Stdout, os.Stderr))
}
