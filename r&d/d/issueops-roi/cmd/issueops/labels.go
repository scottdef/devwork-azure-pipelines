package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"

	"github.com/CoolEngOrg/issueops/internal/ghapi"
)

type labelDef struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// cmdLabels creates or updates the platform labels from config/labels.json.
//
//	issueops labels [--file config/labels.json] [--dry-run]
func cmdLabels(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("labels", flag.ExitOnError)
	var c common
	c.register(fs)
	file := fs.String("file", "config/labels.json", "label definitions")
	dry := fs.Bool("dry-run", false, "print what would change")
	_ = fs.Parse(args)
	var defs []labelDef
	if err := readJSON(*file, &defs); err != nil {
		return err
	}
	if *dry {
		c.offline = true
	}
	a, err := c.load()
	if err != nil {
		return err
	}
	for _, d := range defs {
		if *dry {
			fmt.Printf("would sync %-28s #%s  %s\n", d.Name, d.Color, d.Description)
			continue
		}
		if a.gh == nil {
			return errors.New("labels: the GitHub API is required")
		}
		base := fmt.Sprintf("repos/%s/%s/labels", a.reg.Organization, a.reg.Repository)
		resp, err := a.gh.JSON(ctx, ghapi.Request{Method: http.MethodPatch, Path: base + "/" + url.PathEscape(d.Name),
			Body: map[string]string{"color": d.Color, "description": d.Description}, OK: []int{http.StatusNotFound}}, nil)
		if err != nil {
			return fmt.Errorf("labels: update %s: %w", d.Name, err)
		}
		if resp.Status == http.StatusNotFound {
			if _, err := a.gh.JSON(ctx, ghapi.Request{Method: http.MethodPost, Path: base, Body: d}, nil); err != nil {
				return fmt.Errorf("labels: create %s: %w", d.Name, err)
			}
			fmt.Println("created", d.Name)
			continue
		}
		fmt.Println("updated", d.Name)
	}
	return nil
}
