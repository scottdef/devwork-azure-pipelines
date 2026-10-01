package clone

import (
	"context"
	"strings"

	"github.com/coolado/adoclone/internal/jx"
)

var defaultViews = map[string]bool{"local": true, "prerelease": true, "release": true}

// CloneFeeds copies project-scoped Artifacts feeds: settings, views, upstream
// sources and explicit permissions. Organization-scoped feeds need nothing.
// Packages can't be copied through the REST API, so feeds that have packages
// get a todo.json entry to re-publish them with the package manager clients.
func CloneFeeds(ctx context.Context, x *Ctx) error {
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	feeds, err := valueOf[jx.M](ctx, x, x.srcFeeds("_apis/packaging/feeds?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(feeds))
	have := map[string]string{}
	if t, err := valueOf[jx.M](ctx, x, x.tgtFeeds("_apis/packaging/feeds?api-version=7.1")); err == nil {
		for _, f := range t {
			have[fold(jx.Str(f, "name"))] = jx.Str(f, "id")
		}
	}
	type pending struct {
		tgtID string
		ups   []any
	}
	var later []pending
	for _, f := range feeds {
		fid, name := jx.Str(f, "id"), jx.Str(f, "name")
		tid, ok := x.mapped("feed", fid)
		if !ok {
			tid, ok = have[fold(name)]
		}
		var deferred []any
		if !ok {
			var keep []any
			for _, u := range jx.Arr(f, "upstreamSources") {
				m := jx.Copy(u).(jx.M)
				jx.Del(m, "id", "status", "statusDetails", "deletedDate")
				// Upstreams into another feed of the source project wait until
				// that feed exists in the target.
				if strings.EqualFold(jx.Str(m, "internalUpstreamProjectId"), src.ID) {
					deferred = append(deferred, m)
					continue
				}
				keep = append(keep, m)
			}
			body := jx.M{"name": name, "upstreamSources": keep}
			for _, k := range []string{"description", "upstreamEnabled", "hideDeletedPackageVersions", "badgesEnabled"} {
				if v, ok := f[k]; ok {
					body[k] = v
				}
			}
			var created jx.M
			if err := x.C.Post(ctx, x.tgtFeeds("_apis/packaging/feeds?api-version=7.1"), body, &created); err != nil {
				x.fail("feed "+name, err)
				continue
			}
			x.wrote()
			tid = jx.Str(created, "id")
		}
		if x.Dry() {
			continue
		}
		x.record("feed", strings.ToLower(fid), tid)
		if len(deferred) > 0 {
			later = append(later, pending{tid, deferred})
		}
		if err := x.copyFeedViews(ctx, fid, tid); err != nil {
			x.fail("views of feed "+name, err)
		}
		if err := x.copyFeedPermissions(ctx, fid, tid); err != nil {
			x.fail("permissions of feed "+name, err)
		}
		pkgs, err := valueOf[jx.M](ctx, x, x.srcFeeds("_apis/packaging/Feeds/"+fid+"/packages?$top=1&api-version=7.1"))
		if err == nil && len(pkgs) > 0 {
			x.todo("feed "+name, "re-publish its packages to the target feed with the package manager client (packages can't be copied through the API)")
		}
	}
	gm := x.guidMap()
	for _, p := range later {
		var cur jx.M
		if err := x.C.Get(ctx, x.tgtFeeds("_apis/packaging/feeds/"+p.tgtID+"?api-version=7.1"), &cur); err != nil {
			x.fail("upstreams of feed "+p.tgtID, err)
			continue
		}
		ups := jx.Arr(cur, "upstreamSources")
		for _, u := range p.ups {
			ups = append(ups, jx.RemapJSON(u, gm))
		}
		if err := x.C.Patch(ctx, x.tgtFeeds("_apis/packaging/feeds/"+p.tgtID+"?api-version=7.1"), jx.M{"upstreamSources": ups}, nil); err != nil {
			x.fail("upstreams of feed "+p.tgtID, err)
		}
	}
	return nil
}

func (x *Ctx) copyFeedViews(ctx context.Context, srcFeed, tgtFeed string) error {
	views, err := valueOf[jx.M](ctx, x, x.srcFeeds("_apis/packaging/Feeds/"+srcFeed+"/views?api-version=7.1"))
	if err != nil {
		return err
	}
	have := map[string]string{}
	if t, err := valueOf[jx.M](ctx, x, x.tgtFeeds("_apis/packaging/Feeds/"+tgtFeed+"/views?api-version=7.1")); err == nil {
		for _, v := range t {
			have[fold(jx.Str(v, "name"))] = jx.Str(v, "id")
		}
	}
	for _, v := range views {
		name := jx.Str(v, "name")
		if id, ok := have[fold(name)]; ok {
			x.record("feedView", strings.ToLower(jx.Str(v, "id")), id)
			continue
		}
		if defaultViews[fold(name)] {
			continue
		}
		var created jx.M
		body := jx.M{"name": name, "type": v["type"], "visibility": v["visibility"]}
		if err := x.C.Post(ctx, x.tgtFeeds("_apis/packaging/Feeds/"+tgtFeed+"/views?api-version=7.1"), body, &created); err != nil {
			return err
		}
		x.record("feedView", strings.ToLower(jx.Str(v, "id")), jx.Str(created, "id"))
	}
	return nil
}

func (x *Ctx) copyFeedPermissions(ctx context.Context, srcFeed, tgtFeed string) error {
	perms, err := valueOf[jx.M](ctx, x, x.srcFeeds("_apis/packaging/Feeds/"+srcFeed+"/permissions?api-version=7.1"))
	if err != nil {
		return err
	}
	sg, err := x.sourceGroups(ctx)
	if err != nil {
		return err
	}
	idd, gm := lowerKeys(x.S.Map("identityDescriptor")), x.guidMap()
	mapDesc := func(d string) (string, bool) { return x.mapDescriptor(d, idd, gm, sg) }
	var body []any
	for _, p := range perms {
		if inh, _ := p["isInheritedRole"].(bool); inh {
			continue
		}
		d, ok := remapDescriptor(p["identityDescriptor"], mapDesc)
		if !ok {
			x.todo("feed permission "+jx.Str(p, "displayName"), "a source project identity with no target equivalent; grant the right target identity by hand")
			continue
		}
		body = append(body, jx.M{"identityDescriptor": d, "role": p["role"]})
	}
	if len(body) == 0 {
		return nil
	}
	return x.C.Patch(ctx, x.tgtFeeds("_apis/packaging/Feeds/"+tgtFeed+"/permissions?api-version=7.1"), body, nil)
}

// remapDescriptor maps an identity descriptor given either as
// "type;identifier" or as {identityType, identifier}.
func remapDescriptor(v any, mapDesc func(string) (string, bool)) (any, bool) {
	switch d := v.(type) {
	case string:
		if d == "" {
			return nil, false
		}
		return mapDesc(d)
	case map[string]any:
		t, ok := mapDesc(jx.Str(d, "identityType") + ";" + jx.Str(d, "identifier"))
		if !ok {
			return nil, false
		}
		typ, id, _ := strings.Cut(t, ";")
		return jx.M{"identityType": typ, "identifier": id}, true
	}
	return nil, false
}
