package main

import (
	"fmt"
	"log"

	"github.com/scraperwiki/git-prep-directory"

	"github.com/codegangsta/cli"
)

func init() {
	log.SetPrefix("")
}

func main() {
	app := cli.NewApp()
	app.Name = "git-prep-directory"
	app.Version = "1.0"
	app.Usage = "Build tools friendly way of repeatedly cloning a git\n" +
		"   repository using a submodule cache and setting file timestamps to commit times."

	app.Action = actionMain

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "url, u",
			Usage: "URL to clone",
		},
		cli.StringFlag{
			Name:  "ref, r",
			Usage: "ref to checkout",
		},
		cli.StringFlag{
			Name:  "destination, d",
			Usage: "destination dir",
			Value: "./src",
		},
	}

	app.RunAndExitOnError()
}

func actionMain(c *cli.Context) {
	if !c.GlobalIsSet("url") || !c.GlobalIsSet("ref") {
		log.Fatalln("Error: --url and --ref required")
	}

	where, err := git.PrepBuildDirectory(
		c.GlobalString("destination"),
		c.GlobalString("url"),
		c.GlobalString("ref"))
	if err != nil {
		log.Fatalln("Error:", err)
	}
	log.Printf("Checked out %v at %v", where.Name, where.Dir)
	fmt.Println(where.Dir)
}
