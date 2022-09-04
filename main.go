package main

import (
	"github.com/davecgh/go-spew/spew"
	"gitlab.lrz.de/bbrft/cli"
)

func main() {
	args := cli.ParseArgs()
	spew.Dump(args)
}
