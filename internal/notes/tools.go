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
			"never the conversation or a summary of it. A capture is a note or an idea, never established knowledge or documentation. "+
			"text is what to keep, in Markdown, and becomes the note's body. title is a short title, and may be left out. "+
			`kind is "idea" when the owner calls it an idea, and is otherwise left out, which makes it a note. `+
			"area is the slug or the name of the area the owner named, as list_areas gives them, and the note is filed there; "+
			"an area that does not exist is refused, and nothing is saved. "+
			"When the owner named no area and did not call it an idea, leave area and kind out: the note waits in the inbox, where the owner files it. "+
			"An idea with no area is kept in the personal area, outside the inbox. "+
			"tags are words to find the note by, each with no spaces. "+
			"said is the owner's own words, word for word as you heard them, not a summary, and may be left out. "+
			"text and said each have a cap on their length: one over it is refused with a line that names the cap, and nothing is saved. "+
			"Then ask the owner what to keep, and never split a text over several notes to get it through. "+
			"Gives the note as list_notes lists it, with its ref (notes:ID).",
			module.Write, n.createNote),
		module.NewTool("list_notes", "List the owner's notes and ideas, the most recently changed first, and total, how many match in all. "+
			"Use it to find a note to read or change, or to see what waits in the inbox; to find notes by what they say, use search. "+
			"Every filter may be left out. area is the slug or the name of one area, as list_areas gives them. "+
			`kind is "note" or "idea". inbox true gives only the captures that wait in the inbox, and false only the notes that are filed. `+
			"tag gives only the notes with that tag. limit is how many to give, 50 unless given and 500 at most. "+
			"Each note has its ref (notes:ID), the slug of its area, its kind, its title, the first 200 characters of its body as excerpt, "+
			"its inbox flag, its tags, how many files it has, its revision, the source of its last change, and when it was made and last changed. "+
			"A note in the trash is never listed.",
			module.Read, n.listNotes),
		module.NewTool("get_note", "Read one of the owner's notes in full, by its ref (notes:ID), as list_notes and search give it. "+
			"Gives what list_notes gives of it and its whole body; said, the owner's own words its capture carried, or empty; "+
			"its links and the items that link to it, each with its ref, kind, state, title and whether it is available; "+
			"and its attached files, each with its id, name, type and size. A file's contents cannot be read through a tool. "+
			"Read a note before you change it with update_note, which takes its revision. A note in the trash is not found.",
			module.Read, n.getNote),
		module.NewTool("update_note", "Change one of the owner's notes, by its ref (notes:ID), when the owner asks you to. Give only what changes. "+
			"revision is the note's revision as you last read it. A change made from an older one is refused: "+
			"read the note again with get_note, and make the change on what it says now. "+
			"append is text to add to the end of the body, as a paragraph of its own, and needs no revision when it is all the change gives: "+
			"use it to add to a note. body replaces the whole body. Give body or append, never both. "+
			`title, kind ("note" or "idea"), area (a slug or a name, as list_areas gives them), tags, and links `+
			"(refs such as notes:42, at most 100) each replace what the note has. "+
			"A change of area, kind or tags files a note that waits in the inbox, unless inbox is true too; "+
			"inbox true puts the note in the inbox, in the personal area, and false files it where it is. "+
			"body and append each have the cap on their length that text of create_note has: one over it is refused with a line that names the cap, "+
			"and nothing changes. Never split a text over several changes to get it through. "+
			"Every change is kept in the note's history. Gives the note in full, as get_note does. A note in the trash is not found.",
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
