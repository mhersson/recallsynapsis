package notes

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// tools are the module's MCP tools. Their descriptions carry all the guidance
// a client gets, since a gateway such as Portal relays neither annotations
// nor instructions.
func (n notes) tools() []module.Tool {
	return []module.Tool{
		module.NewTool("create_note", "Save a note or an idea the owner asked you to keep. "+
			"Use it only when the owner asks you to note, keep, save or remember something, and save only what they asked to keep, "+
			"never the conversation or a summary of it. text is what to keep, in Markdown. "+
			`kind is "idea" when the owner calls it an idea, and is otherwise left out. `+
			"area is the project or area the owner named, by its slug or name as list_areas gives them; when they named none, leave it out. "+
			"tags are single words to find the note by. said is the owner's own words, word for word, not a summary, and may be left out. "+
			"If a text is refused as too long, ask the owner what to keep, and never split it over several notes.",
			module.Write, n.createNote),
		module.NewTool("list_notes", "List the owner's notes and ideas, the most recently changed first: "+
			"to find a note to read or change, or to see what waits in the inbox. To find notes by what they say, use search. "+
			`area is one project or area, by its slug or name as list_areas gives them, kind is "note" or "idea", `+
			"and tag gives the notes with that tag. inbox true gives only the captures that wait in the inbox, and false only the filed notes. "+
			"limit is how many to give, 50 unless given, and total says how many match in all.",
			module.Read, n.listNotes),
		module.NewTool("get_note", "Read one of the owner's notes in full, by its ref (notes:ID), as list_notes and search give it: "+
			"its whole body, its links and what links to it, and the names of its files, whose contents no tool reads. "+
			"Read a note before you change it with update_note, which takes its revision.",
			module.Read, n.getNote),
		module.NewTool("update_note", "Change one of the owner's notes, by its ref (notes:ID), when the owner asks you to. Give only what changes. "+
			"To add to a note, give append alone, which needs no revision. "+
			"Any other change gives revision, the note's revision as you read it with get_note: "+
			"if the call is refused because the note has changed since, read it again and make the change on what it says now. "+
			"body replaces the whole body, and title, kind, area (the project or area it moves to, by its slug or name), tags "+
			"and links (refs such as notes:42) each replace what the note has. "+
			"A change of area, kind or tags files a note that waits in the inbox; inbox false files it where it is, and true puts it back there. "+
			"If a text is refused as too long, ask the owner what to keep.",
			module.Write, n.updateNote),
		module.NewTool("trash_note", "Move one of the owner's notes to the trash, by its ref (notes:ID), only when the owner asks you to remove it. "+
			"No tool finds the note after that: it leaves every list and search, and no tool restores it or deletes it for good. "+
			"Gives the note as list_notes lists it, with when it was trashed.",
			module.Write, n.trashNote),
	}
}

// captureIn is what create_note takes.
type captureIn struct {
	Text  string   `json:"text"`
	Title string   `json:"title,omitempty"`
	Kind  string   `json:"kind,omitempty"`
	Area  string   `json:"area,omitempty"`
	Tags  []string `json:"tags,omitempty"`
	Said  string   `json:"said,omitempty"`
}

// listIn is what list_notes takes.
type listIn struct {
	Area  string `json:"area,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Inbox *bool  `json:"inbox,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// refIn is what a tool of one note takes.
type refIn struct {
	Ref module.Ref `json:"ref"`
}

// updateIn is what update_note takes: the note, and the change of it.
type updateIn struct {
	Ref module.Ref `json:"ref"`
	change
}

// createNote files the note in the area it names, keeps an idea that names
// none in the personal area, and puts any other in the inbox.
func (n notes) createNote(ctx context.Context, in captureIn) (row, error) {
	limit := n.settings.CaptureMaxChars
	if err := cmp.Or(module.Capped("text", &in.Text, limit), module.Capped("said", &in.Said, limit)); err != nil {
		return row{}, err
	}

	out, err := n.create(ctx, draft{
		Title: in.Title, Body: in.Text, Kind: in.Kind, Area: in.Area, Tags: in.Tags,
		Inbox: in.Area == "" && in.Kind != "idea", Said: in.Said,
	})

	return out.row, err
}

func (n notes) listNotes(ctx context.Context, in listIn) (page, error) {
	return n.list(ctx, filter{area: in.Area, kind: in.Kind, inbox: in.Inbox, tag: in.Tag, limit: in.Limit})
}

func (n notes) getNote(ctx context.Context, in refIn) (note, error) {
	id, err := n.open(ctx, in.Ref)
	if err != nil {
		return note{}, err
	}

	return n.get(ctx, id)
}

func (n notes) updateNote(ctx context.Context, in updateIn) (note, error) {
	limit := n.settings.CaptureMaxChars
	if err := cmp.Or(module.Capped("body", in.Body, limit), module.Capped("append", in.Append, limit)); err != nil {
		return note{}, err
	}

	id, err := n.open(ctx, in.Ref)
	if err != nil {
		return note{}, err
	}

	return n.change(ctx, id, in.change)
}

func (n notes) trashNote(ctx context.Context, in refIn) (row, error) {
	id, err := n.open(ctx, in.Ref)
	if err != nil {
		return row{}, err
	}

	out, err := n.trash(ctx, id, true)

	return out.row, err
}

// open gives the id of the caller's note that ref names. No tool finds a note
// in the trash, so one there is not found, as an absent one is.
func (n notes) open(ctx context.Context, ref module.Ref) (int64, error) {
	mod, id, ok := ref.Split()
	if !ok || mod != name {
		return 0, fmt.Errorf("the ref %q is not a note's, which is notes:ID: %w", ref, store.ErrInvalid)
	}

	c, _ := module.CallerFrom(ctx)

	_, err := changeable(ctx, n.Store, c.UserID, id)
	if errors.Is(err, errTrashed) {
		err = fmt.Errorf("note %d: %w", id, store.ErrNotFound)
	}

	return id, err
}
