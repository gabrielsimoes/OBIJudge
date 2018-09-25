# OBIJudge

OBIJudge is a programming competitions judge designed to be run in the competitor's
machine. It features a sandboxing method based on IOI's [isolate](https://github.com/ioi/isolate),
using Linux's Cgroups, and therefore needs root permissions to work.

![Screenshot](screenshot.png)

## Installation instructions

You can simply clone the repository inside your `GOPATH` and run the following:

```bash
go get .
yarn install
make static
make generate-statics-sources # if you want static assets to be bound to the binary
make reference # if you want to be able to access programming language reference inside the web interface
make build
```

A binary will be created in the folder. Usage instructions below assume
this binary is called `OBIJudge`.

## Usage instructions

OBIJudge is programmed to run program executions on the `/obijudge` folder of your
system. Make sure that such folder is not used for other things.

Use `./OBIJudge builddb` to build a `.zip` file containing the contest data.
Usage instructions are available by calling `./OBIJudge builddb -h`.

Use `./OBIJudge run` to run an http server and run the contest. Usage
instructions are available by calling `./OBIJudge run -h`.

To build the sample contest database and run the web interface:

```bash
./OBIJudge builddb
sudo ./OBIJudge run
```

Then access `localhost` in your web browser, and use the contest database
file just created and the password used to access the contest.
