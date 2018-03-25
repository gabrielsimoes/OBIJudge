package main

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/urfave/cli.v1"
)

func main() {
	app := cli.NewApp()
	app.Name = "obijudge"
	app.Version = "X"
	app.Usage = "A local programming contests judge"
	app.Authors = []cli.Author{
		{Name: "Gabriel Simões", Email: "simoes.sgabriel@gmail.com"},
	}
	app.EnableBashCompletion = true
	app.Commands = []cli.Command{
		{
			Name:  "run",
			Usage: "Run obijudge server",
			Flags: []cli.Flag{
				cli.UintFlag{
					Name:  "port, p",
					Usage: "Port where interface will listen (localhost-only)",
					Value: 8080,
				},
				cli.StringFlag{
					Name:  "database, d",
					Usage: "Contests database file",
					Value: "contests.zip",
				},
				cli.StringFlag{
					Name:  "reference, r",
					Usage: "File where language reference is stored",
					Value: "reference.zip",
				},
			},
			Action: func(c *cli.Context) error {
				runJudge()
				defer close(submissions)
				err := runServer(c.String("database"), c.String("reference"), c.Uint("port"))
				if err != nil {
					fmt.Println(err)
				}
				return err
			},
		},
		{
			Name:  "builddb",
			Usage: "Build contests database file",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "source, s",
					Usage: "Folder where contests data is located",
					Value: "contests/",
				},
				cli.StringFlag{
					Name:  "target, t",
					Usage: "File where the database will be created (erases if already exists)",
					Value: "contests.zip",
				},
				cli.StringFlag{
					Name:  "pass, p",
					Usage: "16 letters password to encrypt database (will generate one if empty)",
				},
			},
			Action: func(c *cli.Context) error {
				err := buildDatabase(c.String("source"), c.String("target"),
					[]byte(c.String("pass")))
				return err
			},
		},
	}

	if len(os.Args) == 1 {
		fmt.Printf("Executing \"run\" command with default options, append \"help\" to learn usage.\n")
		err := app.Run([]string{"obijudge", "run"})
		if err != nil {
			log.Fatal(err)
		}
	} else {
		err := app.Run(os.Args)
		if err != nil {
			log.Fatal(err)
		}
	}
}
