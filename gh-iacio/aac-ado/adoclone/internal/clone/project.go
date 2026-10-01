package clone

import (
	"context"
	"fmt"
	"strings"

	"github.com/coolado/adoclone/internal/client"
)

// Project is the subset of a team project the cloners use.
type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	State       string `json:"state"`
	DefaultTeam struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"defaultTeam"`
	Capabilities struct {
		Versioncontrol struct {
			SourceControlType string `json:"sourceControlType"`
		} `json:"versioncontrol"`
		ProcessTemplate struct {
			TemplateTypeID string `json:"templateTypeId"`
		} `json:"processTemplate"`
	} `json:"capabilities"`
}

func getProject(ctx context.Context, c *client.Client, name string) (*Project, error) {
	var p Project
	if err := c.Get(ctx, c.URL("", "_apis/projects/"+client.P(name)+"?includeCapabilities=true&api-version=7.1"), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// SrcProject returns the source project (cached).
func (x *Ctx) SrcProject(ctx context.Context) (*Project, error) {
	if x.srcProj != nil {
		return x.srcProj, nil
	}
	p, err := getProject(ctx, x.C, x.Src)
	if err != nil {
		return nil, fmt.Errorf("source project %q: %w", x.Src, err)
	}
	if cur := x.S.SourceProjectID; cur != "" && !strings.EqualFold(cur, p.ID) {
		return nil, fmt.Errorf("this checkpoint was made for source project %s, but %q is %s; use a different -state file", cur, x.Src, p.ID)
	}
	x.srcProj = p
	if !x.Dry() && x.S.SourceProjectID == "" {
		_ = x.S.SetProjects(p.ID, "")
	}
	return p, nil
}

// TgtProject returns the target project (cached). In a dry run against a
// target that doesn't exist yet it returns a placeholder with an empty ID.
func (x *Ctx) TgtProject(ctx context.Context) (*Project, error) {
	if x.tgtProj != nil {
		return x.tgtProj, nil
	}
	p, err := getProject(ctx, x.C, x.Tgt)
	if err != nil {
		if client.IsNotFound(err) {
			if x.Dry() {
				return &Project{Name: x.Tgt}, nil
			}
			return nil, fmt.Errorf("target project %q doesn't exist yet; run the project component first", x.Tgt)
		}
		return nil, err
	}
	if err := x.claimTarget(p); err != nil {
		return nil, err
	}
	x.tgtProj = p
	return p, nil
}

// claimTarget makes sure an existing target project is the one this
// checkpoint created, so a mistyped -target can't make adoclone push into,
// or overwrite permissions on, somebody else's project. -adopt-existing
// accepts a project the checkpoint doesn't know yet.
func (x *Ctx) claimTarget(p *Project) error {
	if x.Dry() {
		return nil
	}
	switch cur := x.S.TargetProjectID; {
	case cur != "" && !strings.EqualFold(cur, p.ID):
		return fmt.Errorf("this checkpoint belongs to target project %s, but %q is %s; use a different -state file", cur, x.Tgt, p.ID)
	case cur == "" && !x.Opt.AdoptExisting:
		return fmt.Errorf("target project %q already exists and this checkpoint didn't create it; choose a new -target, or pass -adopt-existing to copy into it", x.Tgt)
	case cur == "":
		return x.S.SetProjects("", p.ID)
	}
	return nil
}

// CloneProject creates the target project with the source's process and
// source-control type. Visibility defaults to private: public projects are
// being retired, so -visibility=source is opt-in.
func CloneProject(ctx context.Context, x *Ctx) error {
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	x.read(1)
	if p, err := getProject(ctx, x.C, x.Tgt); err == nil {
		if err := x.claimTarget(p); err != nil {
			return err
		}
		x.tgtProj = p
		x.Log.Info("target project already exists", "id", p.ID)
		return nil
	} else if !client.IsNotFound(err) {
		return err
	}
	vis := x.Opt.Visibility
	if vis == "" || vis == "source" {
		vis = src.Visibility
	}
	scm := src.Capabilities.Versioncontrol.SourceControlType
	if scm == "" {
		scm = "Git"
	}
	body := map[string]any{
		"name":        x.Tgt,
		"description": src.Description,
		"visibility":  vis,
		"capabilities": map[string]any{
			"versioncontrol":  map[string]string{"sourceControlType": scm},
			"processTemplate": map[string]string{"templateTypeId": src.Capabilities.ProcessTemplate.TemplateTypeID},
		},
	}
	var op struct {
		ID string `json:"id"`
	}
	if err := x.C.Post(ctx, x.org("_apis/projects?api-version=7.1"), body, &op); err != nil {
		return err
	}
	if x.Dry() {
		return nil
	}
	if err := waitOperation(ctx, x, op.ID); err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	p, err := getProject(ctx, x.C, x.Tgt)
	if err != nil {
		return err
	}
	x.tgtProj = p
	x.wrote()
	return x.S.SetProjects(src.ID, p.ID)
}
