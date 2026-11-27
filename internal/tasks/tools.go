package tasks

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// tools are the module's MCP tools. Their descriptions carry all the guidance
// a client gets, since a gateway such as Portal relays neither annotations
// nor instructions.
func (t tasks) tools() []module.Tool {
	return []module.Tool{
		module.NewTool("create_tasks", "Save tasks the owner asked you to keep: things they mean to do. "+
			"Use it when the owner asks you to add, plan or remember something to do, and save only what they asked for, "+
			"never the conversation or a summary of it. Several tasks said at once go in one call, which saves all or none. "+
			"Each task has a title, a short line, and may have notes, more on what to do. "+
			"area is the project or area the owner named, by its slug or name as list_areas gives them, and is left out when they named none. "+
			"A date is YYYY-MM-DD and a time HH:MM on a 24-hour clock. planned_from and planned_to are the days the owner plans to do it: "+
			"both for a period, such as a weekend, and planned_from alone for one day. "+
			"planned_time is when it starts and deadline the day it must be done by, each given only when the owner named one. "+
			`Turn what the owner said, such as "next weekend", into dates counted from today, and keep the period they gave: `+
			"invent no day, time or deadline, and ask when you are unsure which day they mean. "+
			`list is "soon" or "someday" when the owner said so; when they said nothing of when, give no list and no date, `+
			"and the task waits for them to sort. links are refs of items the task concerns, such as notes:42. "+
			"said is the owner's own words, word for word, not a summary, and may be left out. "+
			"today is the date you counted from, and is given whenever a date is: if the call is refused because it is another day, "+
			"the line gives the right one, so count again from it and call again. "+
			"If a text is refused as too long, ask the owner what to keep, and never split it over several tasks. "+
			"Read the dates back to the owner with the weekdays the answer gives beside them.",
			module.Write, t.createTasks),
		module.NewTool("list_tasks", "List the owner's tasks: to see what a day, a week or a weekend holds, what is on a list, "+
			"what waits to be sorted or what is done, or to find a task to read or change. To find tasks by what they say, use search. "+
			"With no filter it gives the open tasks of every area. area is one project or area, by its slug or name as list_areas gives them. "+
			`list is "soon", "someday", or "none" for the tasks on neither. `+
			"from and to are a span of dates, YYYY-MM-DD, and give the tasks planned or due within it; "+
			"to alone gives everything planned or due up to that day, what is left from earlier too. "+
			"Count the span from today: every answer gives now, and when its date is not the day you counted from, count again and list again. "+
			"unsorted true gives only the tasks that wait to be sorted, and done true the done tasks in place of the open ones. "+
			"limit is how many to give, 50 unless given, and total says how many match in all. "+
			"Tell the owner a date with the weekday given beside it.",
			module.Read, t.listTasks),
		module.NewTool("get_task", "Read one of the owner's tasks in full, by its ref (tasks:ID), as list_tasks and search give it: "+
			"its notes, its links and what links to it, its log of changes, and now, which day it is.",
			module.Read, t.getTask),
		module.NewTool("update_task", "Change one of the owner's tasks, by its ref (tasks:ID), when the owner asks you to: "+
			"plan it again, put it on a list, mark it done or open it again. Give only what changes. "+
			`title, notes, area (the project or area it moves to, by its slug or name), list ("soon", "someday" or "" for none) `+
			"and links each replace what the task has, and done true marks it done and false opens it again. "+
			"A date is YYYY-MM-DD and a time HH:MM on a 24-hour clock. "+
			"planned_from alone plans that one day, and planned_to alone moves the plan's last day. "+
			`"" clears planned_to, planned_time or deadline, and planned_from "" clears the whole plan. `+
			"Turn what the owner said into dates counted from today, and keep the period they gave: invent no day, time or deadline, "+
			"and ask when it is unclear. "+
			"today is the date you counted from, and is given whenever a date is set: if the call is refused because it is another day, "+
			"the line gives the right one, so count again from it and call again. "+
			"If notes is refused as too long, ask the owner what to keep. "+
			"Read the dates back to the owner with the weekdays the answer gives beside them.",
			module.Write, t.updateTask),
		module.NewTool("trash_task", "Move one of the owner's tasks to the trash, by its ref (tasks:ID), only when the owner asks you to remove it; "+
			"to mark one done, use update_task. "+
			"No tool finds the task after that: it leaves every list and search, and no tool restores it or deletes it for good. "+
			"Gives the task as list_tasks lists it, with when it was trashed.",
			module.Write, t.trashTask),
	}
}

// createIn is what create_tasks takes: the tasks, and the date the client
// takes for today.
type createIn struct {
	batch
	Today string `json:"today,omitempty"`
}

// listIn is what list_tasks takes.
type listIn struct {
	Area     string `json:"area,omitempty"`
	List     string `json:"list,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	Unsorted bool   `json:"unsorted,omitempty"`
	Done     bool   `json:"done,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// refIn is what a tool of one task takes.
type refIn struct {
	Ref module.Ref `json:"ref"`
}

// updateIn is what update_task takes: the task, the date the client takes for
// today, and the change of the task.
type updateIn struct {
	Ref   module.Ref `json:"ref"`
	Today string     `json:"today,omitempty"`
	change
}

func (t tasks) createTasks(ctx context.Context, in createIn) (made, error) {
	limit := t.settings.CaptureMaxChars
	if err := module.Capped("said", &in.Said, limit); err != nil {
		return made{}, err
	}

	var dates []*string

	for i := range in.Tasks {
		d := &in.Tasks[i]
		if err := module.Capped("notes", &d.Notes, limit); err != nil {
			return made{}, fmt.Errorf("task %d of the call: %w", i+1, err)
		}

		dates = append(dates, &d.PlannedFrom, &d.PlannedTo, &d.Deadline)
	}

	if err := t.dated(in.Today, dates...); err != nil {
		return made{}, err
	}

	return t.create(ctx, in.batch)
}

func (t tasks) listTasks(ctx context.Context, in listIn) (page, error) {
	return t.list(ctx, filter{area: in.Area, list: in.List, from: in.From, to: in.To, unsorted: in.Unsorted, done: in.Done, limit: in.Limit})
}

func (t tasks) getTask(ctx context.Context, in refIn) (task, error) {
	id, err := t.open(ctx, in.Ref)
	if err != nil {
		return task{}, err
	}

	return t.get(ctx, id)
}

func (t tasks) updateTask(ctx context.Context, in updateIn) (task, error) {
	limit := t.settings.CaptureMaxChars
	if err := cmp.Or(module.Capped("notes", in.Notes, limit), t.dated(in.Today, in.PlannedFrom, in.PlannedTo, in.Deadline)); err != nil {
		return task{}, err
	}

	id, err := t.open(ctx, in.Ref)
	if err != nil {
		return task{}, err
	}

	return t.change(ctx, id, in.change)
}

func (t tasks) trashTask(ctx context.Context, in refIn) (row, error) {
	id, err := t.open(ctx, in.Ref)
	if err != nil {
		return row{}, err
	}

	out, err := t.trash(ctx, id, true)

	return out.row, err
}

// open gives the id of the caller's task that ref names. No tool finds a task
// in the trash, and resolve finds none there, as it finds no absent one.
func (t tasks) open(ctx context.Context, ref module.Ref) (int64, error) {
	mod, id, ok := ref.Split()
	if !ok || mod != name {
		return 0, fmt.Errorf("the ref %q is not a task's, which is tasks:ID: %w", ref, store.ErrInvalid)
	}

	_, err := t.resolve(ctx, id)

	return id, err
}

// dated refuses a call that sets one of dates to a day unless today is the
// server's today, so nothing a client counted from a wrong today is saved.
func (t tasks) dated(today string, dates ...*string) error {
	if !slices.ContainsFunc(dates, func(d *string) bool { return d != nil && *d != "" }) {
		return nil
	}

	now := t.today()
	if today == now.Date {
		return nil
	}

	return fmt.Errorf("today is %s, a %s, in the time zone %s, where a week starts on %s: give it as today, and count the dates from it: %w",
		now.Date, now.Weekday, now.Timezone, now.WeekStart, store.ErrInvalid)
}
