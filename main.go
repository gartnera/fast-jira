package main

import (
	"context"
	"os"

	"github.com/alexgartner-bc/fast-jira/background"
	"github.com/andygrunwald/go-jira"
	"go.uber.org/zap"
)

func main() {
	ctx := context.TODO()
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	tp := jira.BasicAuthTransport{
		Username: os.Getenv("JIRA_EMAIL"),
		Password: os.Getenv("JIRA_TOKEN"),
	}
	jiraClient, err := jira.NewClient(tp.Client(), "https://braincorporation.atlassian.net/")
	if err != nil {
		panic(err)
	}
	s := background.Syncer{
		Logger:     logger,
		JiraClient: jiraClient,
	}
	s.Start(ctx)
}
