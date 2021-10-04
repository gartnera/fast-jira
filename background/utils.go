package background

import (
	"time"

	jira "github.com/andygrunwald/go-jira"
)

func jiraTimeFormat(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}

func jiraTimeToString(t jira.Time) string {
	tj, _ := t.MarshalJSON()
	return string(tj)
}
