package main

import "github.com/alecthomas/kong"

var (
	version = "dev"
	commit  = ""
	date    = ""
)

type CLI struct{}

func main() {
	kong.Parse(
		&CLI{},
		kong.Name("tfc"),
		kong.Description("Terraform Cloud API CLI"),
	)
}
