package clone

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/coolado/adoclone/internal/client"
	"github.com/coolado/adoclone/internal/jx"
)

// CloneTeams maps the default team to the target's default team, creates the
// other teams, and copies each team's iterations, area values, settings and
// direct members. Team administrators are copied by security (Identity ACLs).
func CloneTeams(ctx context.Context, x *Ctx) error {
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	srcTeams, err := listTeams(ctx, x, src.ID)
	if err != nil {
		return err
	}
	x.read(len(srcTeams))
	byName := map[string]team{}
	if tgt.ID != "" {
		tgtTeams, err := listTeams(ctx, x, tgt.ID)
		if err != nil {
			return err
		}
		for _, t := range tgtTeams {
			byName[fold(t.Name)] = t
		}
	}
	tgtRef := tgt.ID
	if tgtRef == "" {
		tgtRef = url.PathEscape(x.Tgt)
	}
	for _, t := range srcTeams {
		var tt team
		switch e, ok := byName[fold(t.Name)]; {
		case t.ID == src.DefaultTeam.ID && tgt.DefaultTeam.ID != "":
			tt = team{ID: tgt.DefaultTeam.ID, Name: tgt.DefaultTeam.Name}
		case ok:
			tt = e
		default:
			body := map[string]string{"name": t.Name, "description": t.Description}
			if err := x.C.Post(ctx, x.org("_apis/projects/"+tgtRef+"/teams?api-version=7.1"), body, &tt); err != nil {
				x.fail("team "+t.Name, err)
				continue
			}
			x.wrote()
		}
		if x.Dry() {
			continue
		}
		x.record("team", strings.ToLower(t.ID), tt.ID)
		if err := x.mapTeamGroup(ctx, t.ID, tt.ID); err != nil {
			x.fail("team group "+t.Name, err)
		}
		if err := x.copyTeamSettings(ctx, t.ID, tt.ID); err != nil {
			x.fail("team settings "+t.Name, err)
		}
		if err := x.copyTeamMembers(ctx, src.ID, t.ID, tt.ID); err != nil {
			x.fail("team members "+t.Name, err)
		}
	}
	return nil
}

func (x *Ctx) mapTeamGroup(ctx context.Context, srcTeam, tgtTeam string) error {
	sd, err := x.descriptorOf(ctx, srcTeam)
	if err != nil {
		return err
	}
	td, err := x.descriptorOf(ctx, tgtTeam)
	if err != nil {
		return err
	}
	x.record("teamSubject", sd, td)
	return x.mapSubject(ctx, sd, td)
}

func (x *Ctx) copyTeamSettings(ctx context.Context, srcTeam, tgtTeam string) error {
	st := x.src(srcTeam + "/_apis/work/teamsettings")
	tt := x.tgt(tgtTeam + "/_apis/work/teamsettings")

	// Team iterations first: the default iteration has to be one of them.
	type iter struct {
		ID string `json:"id"`
	}
	srcIts, err := valueOf[iter](ctx, x, st+"/iterations?api-version=7.1")
	if err != nil {
		return err
	}
	tgtIts, err := valueOf[iter](ctx, x, tt+"/iterations?api-version=7.1")
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, it := range tgtIts {
		have[fold(it.ID)] = true
	}
	for _, it := range srcIts {
		tid, ok := x.mapped("classNode", it.ID)
		if !ok {
			x.todo("team iteration "+it.ID, "iteration node wasn't copied; run nodes first")
			continue
		}
		if have[fold(tid)] {
			continue
		}
		if err := x.C.Post(ctx, tt+"/iterations?api-version=7.1", map[string]string{"id": tid}, nil); err != nil {
			return fmt.Errorf("add iteration: %w", err)
		}
		x.wrote()
	}

	var tfv jx.M
	if err := x.C.Get(ctx, st+"/teamfieldvalues?api-version=7.1", &tfv); err != nil {
		return err
	}
	isArea := strings.EqualFold(jx.Str(tfv, "field", "referenceName"), "System.AreaPath")
	rebase := func(s string) string {
		if isArea {
			return jx.RebasePath(s, x.Src, x.Tgt)
		}
		return s
	}
	var values []any
	for _, v := range jx.Arr(tfv, "values") {
		values = append(values, jx.M{"value": rebase(jx.Str(v, "value")), "includeChildren": jx.Get(v, "includeChildren")})
	}
	if err := x.C.Patch(ctx, tt+"/teamfieldvalues?api-version=7.1",
		jx.M{"defaultValue": rebase(jx.Str(tfv, "defaultValue")), "values": values}, nil); err != nil {
		return fmt.Errorf("area values: %w", err)
	}

	var s jx.M
	if err := x.C.Get(ctx, st+"?api-version=7.1", &s); err != nil {
		return err
	}
	body := jx.M{"bugsBehavior": s["bugsBehavior"], "workingDays": s["workingDays"], "backlogVisibilities": s["backlogVisibilities"]}
	if id := jx.Str(s, "backlogIteration", "id"); id != "" {
		if t, ok := x.mapped("classNode", id); ok {
			body["backlogIteration"] = t
		}
	}
	if mac := jx.Str(s, "defaultIterationMacro"); mac != "" {
		body["defaultIterationMacro"] = mac
	} else if id := jx.Str(s, "defaultIteration", "id"); id != "" {
		if t, ok := x.mapped("classNode", id); ok {
			body["defaultIteration"] = t
		}
	}
	if err := x.C.Patch(ctx, tt+"?api-version=7.1", body, nil); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	return nil
}

func (x *Ctx) copyTeamMembers(ctx context.Context, srcProject, srcTeam, tgtTeam string) error {
	type member struct {
		Identity struct {
			ID string `json:"id"`
		} `json:"identity"`
	}
	members, err := valueOf[member](ctx, x, x.org(fmt.Sprintf("_apis/projects/%s/teams/%s/members?$top=10000&api-version=7.1", srcProject, srcTeam)))
	if err != nil {
		return err
	}
	teamDesc, err := x.descriptorOf(ctx, tgtTeam)
	if err != nil {
		return err
	}
	sg, err := x.sourceGroups(ctx)
	if err != nil {
		return err
	}
	subj, gm := x.S.Map("subject"), x.guidMap()
	for _, m := range members {
		sd, err := x.descriptorOf(ctx, m.Identity.ID)
		if err != nil {
			x.fail("team member "+m.Identity.ID, err)
			continue
		}
		md, ok := x.mapMember(sd, subj, gm, sg)
		if !ok {
			x.todo("team member "+sd, "a source project group with no target equivalent; add the right target group by hand")
			continue
		}
		if err := x.C.Put(ctx, x.vssps("_apis/graph/memberships/"+md+"/"+teamDesc+"?api-version=7.1-preview.1"), nil, nil); err != nil {
			x.fail("team member "+m.Identity.ID, err)
			continue
		}
		x.wrote()
	}
	return nil
}

// CloneBoards copies each mapped team's board columns, swimlanes, card
// settings and card rules. Columns and lanes are matched by name because their
// IDs differ between projects.
func CloneBoards(ctx context.Context, x *Ctx) error {
	teams := x.S.Map("team")
	if len(teams) == 0 {
		x.Log.Info("no mapped teams; run teams first")
		return nil
	}
	for srcTeam, tgtTeam := range teams {
		type board struct {
			Name string `json:"name"`
		}
		boards, err := valueOf[board](ctx, x, x.src(srcTeam+"/_apis/work/boards?api-version=7.1"))
		if err != nil {
			x.fail("boards of team "+srcTeam, err)
			continue
		}
		x.read(len(boards))
		for _, b := range boards {
			if err := x.copyBoard(ctx, srcTeam, tgtTeam, b.Name); err != nil {
				x.fail("board "+b.Name+" of team "+srcTeam, err)
				continue
			}
			x.wrote()
		}
	}
	return nil
}

func (x *Ctx) copyBoard(ctx context.Context, srcTeam, tgtTeam, name string) error {
	sb := x.src(srcTeam + "/_apis/work/boards/" + client.P(name))
	tb := x.tgt(tgtTeam + "/_apis/work/boards/" + client.P(name))
	var sc, tc struct {
		Value []jx.M `json:"value"`
	}
	if err := x.C.Get(ctx, sb+"/columns?api-version=7.1", &sc); err != nil {
		return err
	}
	if err := x.C.Get(ctx, tb+"/columns?api-version=7.1", &tc); err != nil {
		return err
	}
	if err := x.C.Put(ctx, tb+"/columns?api-version=7.1", mergeColumns(sc.Value, tc.Value), nil); err != nil {
		return fmt.Errorf("columns: %w", err)
	}
	var sr, tr struct {
		Value []jx.M `json:"value"`
	}
	if err := x.C.Get(ctx, sb+"/rows?api-version=7.1", &sr); err != nil {
		return err
	}
	if err := x.C.Get(ctx, tb+"/rows?api-version=7.1", &tr); err != nil {
		return err
	}
	if err := x.C.Put(ctx, tb+"/rows?api-version=7.1", mergeRows(sr.Value, tr.Value), nil); err != nil {
		return fmt.Errorf("rows: %w", err)
	}
	var cs jx.M
	if err := x.C.Get(ctx, sb+"/cardsettings?api-version=7.1", &cs); err != nil {
		return err
	}
	jx.Del(cs, "_links", "url")
	if err := x.C.Put(ctx, tb+"/cardsettings?api-version=7.1", cs, nil); err != nil {
		return fmt.Errorf("card settings: %w", err)
	}
	var cr jx.M
	if err := x.C.Get(ctx, sb+"/cardrulesettings?api-version=7.1", &cr); err != nil {
		return err
	}
	jx.Del(cr, "_links", "url")
	if err := x.C.Patch(ctx, tb+"/cardrulesettings?api-version=7.1", cr, nil); err != nil {
		return fmt.Errorf("card rules: %w", err)
	}
	return nil
}

// mergeColumns keeps the source layout but reuses the target's column IDs
// (matched by name, or by type for the incoming/outgoing columns). New columns
// are sent without an ID.
func mergeColumns(src, tgt []jx.M) []any {
	byName, byType := map[string]any{}, map[string]any{}
	for _, t := range tgt {
		byName[fold(jx.Str(t, "name"))] = t["id"]
		if ct := jx.Str(t, "columnType"); ct == "incoming" || ct == "outgoing" {
			byType[ct] = t["id"]
		}
	}
	out := make([]any, 0, len(src))
	for _, s := range src {
		c := jx.Copy(s)
		delete(c, "id")
		if id, ok := byName[fold(jx.Str(s, "name"))]; ok {
			c["id"] = id
		} else if id, ok := byType[jx.Str(s, "columnType")]; ok {
			c["id"] = id
		}
		out = append(out, c)
	}
	return out
}

const defaultLane = "00000000-0000-0000-0000-000000000000"

// mergeRows does the same for swimlanes; the default lane keeps its all-zero ID.
func mergeRows(src, tgt []jx.M) []any {
	byName := map[string]any{}
	for _, t := range tgt {
		byName[fold(jx.Str(t, "name"))] = t["id"]
	}
	out := make([]any, 0, len(src))
	for _, s := range src {
		r := jx.Copy(s)
		if jx.Str(s, "id") == defaultLane {
			out = append(out, r)
			continue
		}
		delete(r, "id")
		if id, ok := byName[fold(jx.Str(s, "name"))]; ok {
			r["id"] = id
		}
		out = append(out, r)
	}
	return out
}
