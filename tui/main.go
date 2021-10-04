package main

import (
	"database/sql"
	"fmt"
	"os/exec"

	"github.com/alexgartner-bc/fast-jira/background"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const jiraBaseURL = "https://braincorporation.atlassian.net"

func cycleFocus(app *tview.Application, elements []tview.Primitive, reverse bool) {
	for i, el := range elements {
		if !el.HasFocus() {
			continue
		}

		if reverse {
			i = i - 1
			if i < 0 {
				i = len(elements) - 1
			}
		} else {
			i = i + 1
			i = i % len(elements)
		}

		app.SetFocus(elements[i])
		return
	}
}

func main() {
	db, err := sql.Open("sqlite3", "../jira.db")
	if err != nil {
		panic(err)
	}

	var dbIssues []*background.DbIssue
	dbIssuesIdx := 0

	app := tview.NewApplication()
	detail := tview.NewTextView()
	detail.SetTitle("detail").SetBorder(true)

	table := tview.NewTable()
	table.SetBorder(true)
	table.SetSelectable(true, false)

	grid := tview.NewGrid().SetRows(-1, -1, 1)
	grid = grid.AddItem(table, 1, 0, 1, 1, 0, 100, false)
	grid = grid.AddItem(detail, 0, 0, 1, 1, 0, 100, false)

	input := tview.NewInputField()
	grid = grid.AddItem(input, 2, 0, 1, 1, 0, 100, true)
	app = app.SetFocus(input)

	var pages *tview.Pages

	// Returns a new primitive which puts the provided primitive in the center and
	// sets its size to the given width and height.
	modal := func(p tview.Primitive, width, height int) tview.Primitive {
		return tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(nil, 0, 1, false).
				AddItem(p, height, 1, false).
				AddItem(nil, 0, 1, false), width, 1, false).
			AddItem(nil, 0, 1, false)
	}

	modalExit := func() {
		pages.HidePage("modal")
		app.SetFocus(input)
	}

	openInBrowser := tview.NewButton("Open in browser")
	openInBrowser.SetBorderPadding(1, 1, 1, 1)
	openInBrowser.SetSelectedFunc(func() {
		issue := dbIssues[dbIssuesIdx]
		cmd := exec.Command("xdg-open", fmt.Sprintf("%s/browse/%s", jiraBaseURL, issue.Key))
		err := cmd.Start()
		if err != nil {
			panic(err)
		}
		modalExit()
	})

	exit := tview.NewButton("Exit")
	exit.SetSelectedFunc(modalExit)

	inputs := []tview.Primitive{
		openInBrowser,
		exit,
	}

	modalGrid := tview.NewFlex()
	modalGrid.SetDirection(tview.FlexRow)
	modalGrid.SetBorder(true).SetTitle("Ticket Options")
	modalGrid.AddItem(openInBrowser, 0, 1, true)
	modalGrid.AddItem(exit, 0, 1, true)
	modalGrid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			cycleFocus(app, inputs, false)
		case tcell.KeyBacktab:
			cycleFocus(app, inputs, true)
		case tcell.KeyEscape:
			modalExit()
		}
		return event
	})

	pages = tview.NewPages().
		AddPage("grid", grid, true, true).
		AddPage("modal", modal(modalGrid, 40, 10), true, false)

	detailDisplay := func(idx int) {
		detail.SetText(dbIssues[idx].Render())
		detail.SetTitle(dbIssues[idx].Key)
		detail.ScrollToBeginning()
	}

	tableDisplay := func(idx int, updated bool) {
		if updated {
			table.Clear()
			table.SetCellSimple(0, 0, "key")
			table.SetCellSimple(0, 1, "creator")
			table.SetCellSimple(0, 2, "assignee")
			table.SetCellSimple(0, 3, "type")
			table.SetCellSimple(0, 4, "status")
			table.SetCellSimple(0, 5, "summary")
			table.SetFixed(1, 0)
			for i, issue := range dbIssues {
				ii := i + 1
				table.SetCellSimple(ii, 0, issue.Key)
				table.SetCellSimple(ii, 1, issue.CreatorName)
				table.SetCellSimple(ii, 2, issue.AssigneeName)
				table.SetCellSimple(ii, 3, issue.IssueType)
				table.SetCellSimple(ii, 4, issue.Status)
				table.SetCellSimple(ii, 5, issue.Summary)
			}
		}
		table.ScrollToBeginning()
		table.Select(idx+1, 0)
	}

	noMatch := func() {
		detail.SetTitle("no match")
		detail.SetText("")
		table.Clear()
	}

	input.SetChangedFunc(func(text string) {
		if text == "" {
			return
		}

		query := "SELECT * FROM jira WHERE jira MATCH ? ORDER BY DATETIME(updated) DESC"
		rows, err := db.Query(query, text)
		if err != nil {
			noMatch()
			return
		}
		dbIssues = nil
		for rows.Next() {
			dbIssue := &background.DbIssue{}
			err := rows.Scan(
				&dbIssue.Key, &dbIssue.Summary, &dbIssue.Description, &dbIssue.CreatorEmail, &dbIssue.CreatorName,
				&dbIssue.AssigneeEmail, &dbIssue.AssigneeName, &dbIssue.Comments, &dbIssue.Created, &dbIssue.Updated,
				&dbIssue.FixVersion, &dbIssue.IssueType, &dbIssue.Priority, &dbIssue.Status,
			)
			if err != nil {
				app.Stop()
				panic(err)
			}
			dbIssues = append(dbIssues, dbIssue)
		}
		if len(dbIssues) == 0 {
			noMatch()
			return
		}

		detailDisplay(0)
		tableDisplay(0, true)
		dbIssuesIdx = 0
	})

	grid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			if len(dbIssues) == 0 {
				break
			}
			pages.ShowPage("modal")
			app.SetFocus(modalGrid)
			return nil
		case tcell.KeyTab:
			if dbIssuesIdx < len(dbIssues)-1 {
				dbIssuesIdx += 1
				tableDisplay(dbIssuesIdx, false)
				detailDisplay(dbIssuesIdx)
			}
			return nil
		// shift tab
		case tcell.KeyBacktab:
			if dbIssuesIdx > 0 {
				dbIssuesIdx -= 1
				tableDisplay(dbIssuesIdx, false)
				detailDisplay(dbIssuesIdx)
			}
			return nil
		case tcell.KeyCtrlU:
			currentRow, _ := detail.GetScrollOffset()
			detail.ScrollTo(currentRow-4, 0)
			return nil
		case tcell.KeyCtrlD:
			currentRow, _ := detail.GetScrollOffset()
			detail.ScrollTo(currentRow+4, 0)
			return nil
		}
		return event
	})
	if err := app.SetRoot(pages, true).Run(); err != nil {
		panic(err)
	}
}
