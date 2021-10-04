package background

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/StevenACoffman/j2m"
	jira "github.com/andygrunwald/go-jira"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

type DbIssue struct {
	Key           string
	Summary       string
	Description   string
	CreatorEmail  string
	CreatorName   string
	AssigneeEmail string
	AssigneeName  string
	Comments      string
	Created       string
	Updated       string
	FixVersion    string
	IssueType     string
	Priority      string
	Status        string
}

func (d *DbIssue) Render() string {
	res := ""
	if d.Description != "" {
		res += fmt.Sprintf("Description: %s\n", d.Description)
	}
	if d.FixVersion != "" {
		res += fmt.Sprintf("Fix Version: %s\n", d.FixVersion)
	}
	if d.Priority != "" {
		res += fmt.Sprintf("Priority: %s\n", d.Priority)
	}
	if d.Comments != "" {
		res += "\n"
		res += d.Comments
	}
	return res
}

const schema = `
CREATE VIRTUAL TABLE jira 
USING fts5(,
	key,
	summary, description,
	creatorEmail, creatorName,
	assigneeEmail, assigneeName,
	comments,
	created,
	updated,
	fixVersion,
	issueType,
	priority,
	status,
	tokenize="trigram");
`

type Syncer struct {
	Logger         *zap.Logger
	JiraClient     *jira.Client
	lastUpdateTime time.Time
	db             *sql.DB
}

func (s *Syncer) Start(ctx context.Context) {
	db, err := sql.Open("sqlite3", "./jira.db")
	if err != nil {
		panic(err)
	}
	s.db = db

	_, err = db.ExecContext(ctx, schema)
	if err != nil {
		s.Logger.Info("schema already exists")
	}

	err = s.updateIssues()
	if err != nil {
		s.Logger.Error("failed to updateIssues", zap.Error(err))
	}
	for {
		select {
		case <-time.After(5 * time.Second):
			err := s.updateIssues()
			if err != nil {
				s.Logger.Error("failed to updateIssues", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Syncer) flattenApiComments(jc *jira.Comments) string {
	res := ""
	if jc == nil {
		return res
	}
	res = "---\n"
	for i := len(jc.Comments) - 1; i >= 0; i-- {
		c := jc.Comments[i]
		if c.Author.DisplayName == "Automation for Jira" {
			continue
		}
		res += fmt.Sprintf("Author: %s\n", c.Author.DisplayName)
		res += fmt.Sprintf("Created: %s\n", c.Created)
		res += j2m.JiraToMD(c.Body)
		res += "\n---\n"
	}
	return res
}

func (s *Syncer) flattenFixVersion(fv []*jira.FixVersion) string {
	if fv == nil {
		return ""
	}
	var versions []string
	for _, v := range fv {
		versions = append(versions, v.Name)
	}
	return strings.Join(versions, ", ")
}

func (s *Syncer) apiIssueToDb(ji jira.Issue) DbIssue {
	fields := ji.Fields
	d := DbIssue{
		Key:          ji.Key,
		Summary:      ji.Fields.Summary,
		Description:  ji.Fields.Description,
		CreatorEmail: fields.Creator.EmailAddress,
		CreatorName:  fields.Creator.DisplayName,
		Created:      jiraTimeToString(ji.Fields.Created),
		Updated:      jiraTimeToString(ji.Fields.Updated),
		Comments:     s.flattenApiComments(fields.Comments),
		Priority:     fields.Priority.Name,
		FixVersion:   s.flattenFixVersion(fields.FixVersions),
		IssueType:    fields.Type.Name,
		Status:       fields.Status.Name,
	}
	if fields.Assignee != nil {
		d.AssigneeEmail = fields.Assignee.EmailAddress
		d.AssigneeName = fields.Assignee.DisplayName
	}
	return d
}

func (s *Syncer) writeIssue(ji jira.Issue) error {
	dbIssue := s.apiIssueToDb(ji)
	existingKey := ""
	s.db.QueryRow("SELECT key FROM jira WHERE key=?", dbIssue.Key).Scan(&existingKey)
	if existingKey == "" {
		// insert logic
		_, err := s.db.Exec(
			"INSERT INTO jira VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			dbIssue.Key, dbIssue.Summary, dbIssue.Description,
			dbIssue.CreatorEmail, dbIssue.CreatorName,
			dbIssue.AssigneeEmail, dbIssue.AssigneeName, dbIssue.Comments, dbIssue.Created, dbIssue.Updated,
			dbIssue.FixVersion, dbIssue.IssueType, dbIssue.Priority, dbIssue.Status,
		)
		if err != nil {
			s.Logger.Error("unable to insert", zap.String("key", dbIssue.Key), zap.Error(err))
		}
	} else {
		// update logic
		_, err := s.db.Exec(
			"UPDATE jira SET key=?, summary=?, description=?, creatorEmail=?, creatorName=?, assigneeEmail=?, assigneeName=?, comments=?, created=?, updated=?, fixVersion=?, issueType=?, priority=?, status=? WHERE key=?",
			dbIssue.Key, dbIssue.Summary, dbIssue.Description, dbIssue.CreatorEmail, dbIssue.CreatorName,
			dbIssue.AssigneeEmail, dbIssue.AssigneeName, dbIssue.Comments, dbIssue.Created, dbIssue.Created,
			dbIssue.FixVersion, dbIssue.IssueType, dbIssue.Priority, dbIssue.Status,
			dbIssue.Key,
		)
		if err != nil {
			s.Logger.Error("unable to update", zap.String("key", dbIssue.Key), zap.Error(err))
		}
	}
	return nil
}

func (s *Syncer) updateIssues() error {
	now := time.Now()
	lastUpdateTimeStr := jiraTimeFormat(s.lastUpdateTime)
	newIssuesJql := fmt.Sprintf("project = 'TI' AND updated > '%s'", lastUpdateTimeStr)

	nextIndex := 0
	var issues []jira.Issue
	for {
		opt := &jira.SearchOptions{
			Fields: []string{
				"Creator", "assignee", "created", "creator", "description",
				"fixVersions", "issuelinks", "issuetype", "priority", "project",
				"status", "summary", "updated", "comment", "assignee",
			},
			MaxResults: 1000,
			StartAt:    nextIndex,
		}

		chunk, resp, err := s.JiraClient.Issue.Search(newIssuesJql, opt)
		if err != nil {
			return err
		}

		total := resp.Total
		if issues == nil {
			issues = make([]jira.Issue, 0, total)
		}
		issues = append(issues, chunk...)
		nextIndex = resp.StartAt + len(chunk)
		s.Logger.Debug("issue update response", zap.Int("total", total), zap.Int("nextIndex", nextIndex))
		if nextIndex >= total {
			break
		}
	}

	s.Logger.Info("got new issues", zap.Int("len", len(issues)))
	if len(issues) > 0 {
		s.Logger.Debug("new issue sample", zap.Any("issue", issues[0]))
	}

	for _, ji := range issues {
		err := s.writeIssue(ji)
		if err != nil {
			s.Logger.Error("error writing issue", zap.Any("issue", ji), zap.Error(err))
		}
	}
	s.lastUpdateTime = now
	return nil
}
