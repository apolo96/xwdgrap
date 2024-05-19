package main

import (
	"log/slog"
	"os"

	"github.com/leaanthony/clir"
)

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	gitServerSvc, agentSvc, appConfigSvc, gitRepoSvc, logger, err := serviceProvider()
	slog.SetDefault(logger)
	slog.Info("oko")
	if err != nil {
		println("error: loading service provider")
		return err
	}
	/* CLI */
	cli := clir.NewCli("gitfresh", "A DX Tool to keep the git repositories updated 😎", "v1.0.0")
	flags := &AppFlags{}
	/* Config Command */
	config := cli.NewSubCommand("config", "Configure the application parameters")
	config.AddFlags(flags)
	config.Action(func() error {
		return configCmd(appConfigSvc, flags)
	})
	/* Init Command */
	initCommand := cli.NewSubCommand("init", "Initialise the workspace and agent")
	initCommand.Action(func() error {
		return initCmd(gitRepoSvc, agentSvc, appConfigSvc, gitServerSvc)
	})
	/* Scan Command */
	scan := cli.NewSubCommand("scan", "Discover new repositories to refresh")
	scan.Action(func() error {
		return scanCmd(gitRepoSvc, appConfigSvc, gitServerSvc)
	})
	/* Status Command */
	status := cli.NewSubCommand("status", "Check agent status")
	status.Action(func() error {
		return statusCmd(agentSvc)
	})
	return cli.Run()
}
