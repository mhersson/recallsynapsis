package knowledge

import (
	"context"
	"fmt"
	"time"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// maxSaid caps, in characters, the owner's words update_knowledge keeps.
const maxSaid = 4000

// tools are the module's MCP tools. Their descriptions carry all the guidance
// a client gets, since a gateway such as Portal relays neither annotations
// nor instructions.
func (k knowledge) tools() []module.Tool {
	return []module.Tool{
		module.NewTool("get_knowledge", "Read what is kept of one of the owner's projects: its brief, and the decisions that shape it. "+
			"Read it when a discussion of the project begins, and before you propose a design for it. "+
			"With area, a slug or a name as list_areas gives them, it gives the whole brief and each decision that stands as one line, "+
			"with its day and its ref (knowledge:ID). "+
			"With ref instead, it gives that one decision in full: read a decision's why when the talk touches it. "+
			"With area, full gives every decision in full, for when you need every why, and replaced lists the replaced decisions too.",
			module.Read, k.getKnowledge),
		module.NewTool("update_knowledge", "Record decisions of one of the owner's projects, and only on the owner's word: "+
			"show them the drafted decisions in the conversation first, and call this once they have agreed. "+
			"Record only what shapes the project, the decisions a newcomer must know and that would cost to undo, not every choice of a plan. "+
			"In add, decision is one line of at most 200 characters, why says why and what was turned down, "+
			"source says where it was decided, in words or as an address, and decided is the day, left out for today. "+
			"replaces is the ref (knowledge:ID) of the decision a new one takes the place of: ask the owner whether there is one. "+
			"When only a wording should change, mend the decision instead: its ref, the revision you read with get_knowledge and that ref, "+
			"and the fields to change. One call saves all of it or nothing, and a refusal names the entry. "+
			`A draft the owner says "not now" to may be kept as a note when they ask for that, never as a decision. `+
			"said is the owner's own words agreeing, and may be left out.",
			module.Write, k.updateKnowledge).Asked(),
	}
}

// getIn is what get_knowledge takes: a project, by its slug or its name, or a decision.
type getIn struct {
	Area     string     `json:"area,omitempty"`
	Ref      module.Ref `json:"ref,omitempty"`
	Full     bool       `json:"full,omitempty"`
	Replaced bool       `json:"replaced,omitempty"`
}

// project is what get_knowledge gives of a project: its brief as list_areas
// gives an area it is given, each decision that stands, and the replaced,
// counted and listed when asked for, each a line or, in full, a row.
type project struct {
	Area           string     `json:"area"` // its slug
	Name           string     `json:"name"`
	Brief          string     `json:"brief"`
	BriefRevision  int64      `json:"brief_revision"`
	BriefChangedAt *time.Time `json:"brief_changed_at,omitempty"`
	BriefChangedBy string     `json:"brief_changed_by,omitempty"`
	Decisions      []any      `json:"decisions"`
	ReplacedCount  int        `json:"replaced_count"`
	Replaced       []any      `json:"replaced,omitzero"`
}

// updateIn is what update_knowledge takes: the package, and the owner's words agreeing to it.
type updateIn struct {
	change
	Said string `json:"said,omitempty"`
}

// getKnowledge gives the project in.Area names, or the decision in full that
// in.Ref names; its answers differ in shape, so the tool has no output schema.
func (k knowledge) getKnowledge(ctx context.Context, in getIn) (any, error) {
	if (in.Area == "") == (in.Ref == "") {
		return nil, fmt.Errorf("give area, a project's slug or name, or ref, a decision's knowledge:ID, and not both: %w", store.ErrInvalid)
	}

	if in.Ref != "" {
		id, err := open(in.Ref)
		if err != nil {
			return nil, err
		}

		return k.get(ctx, id)
	}

	c, _ := module.CallerFrom(ctx)

	a, err := k.FindArea(ctx, c.UserID, in.Area)
	if err != nil {
		return nil, err
	}

	l, err := k.list(ctx, a)
	if err != nil {
		return nil, err
	}

	out := project{
		Area: a.Slug, Name: a.Name, Brief: a.Brief, BriefRevision: a.BriefRevision, BriefChangedAt: a.BriefChangedAt,
		BriefChangedBy: a.BriefChangedBy, Decisions: []any{},
	}

	if in.Replaced {
		out.Replaced = []any{}
	}

	for _, r := range l.Decisions {
		var d any = line{r.Ref, r.Decision, r.Decided}
		if in.Full {
			d = r
		}

		if r.ReplacedBy == nil {
			out.Decisions = append(out.Decisions, d)

			continue
		}

		out.ReplacedCount++

		if in.Replaced {
			out.Replaced = append(out.Replaced, d)
		}
	}

	return out, nil
}

// updateKnowledge saves the package as POST /api/knowledge does, and keeps
// said on each revision it writes.
func (k knowledge) updateKnowledge(ctx context.Context, in updateIn) (saved, error) {
	if err := module.Capped("said", &in.Said, maxSaid); err != nil {
		return saved{}, err
	}

	in.change.Said = in.Said
	out, err := k.apply(ctx, in.change)

	return saved{out}, err
}
