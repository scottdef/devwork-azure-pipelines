package clone

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/coolado/adoclone/internal/client"
	"github.com/coolado/adoclone/internal/jx"
)

// Fields the server owns, computes, or that only make sense in the source.
var skipFields = map[string]bool{
	"System.Id": true, "System.Rev": true, "System.AreaId": true, "System.IterationId": true,
	"System.TeamProject": true, "System.NodeName": true, "System.AuthorizedDate": true, "System.RevisedDate": true,
	"System.Watermark": true, "System.CommentCount": true, "System.BoardColumn": true, "System.BoardColumnDone": true,
	"System.BoardLane": true, "System.AuthorizedAs": true, "System.PersonId": true,
	// System.Parent would point at the source parent; hierarchy links are added in pass 2.
	"System.Parent": true,
	// Computed link counters.
	"System.RelatedLinkCount": true, "System.ExternalLinkCount": true, "System.HyperLinkCount": true,
	"System.AttachedFileCount": true, "System.RemoteLinkCount": true,
}

// Fields only settable with bypassRules=true.
var ruleFields = map[string]bool{
	"System.CreatedDate": true, "System.CreatedBy": true, "System.ChangedDate": true, "System.ChangedBy": true,
	"Microsoft.VSTS.Common.StateChangeDate": true, "Microsoft.VSTS.Common.ActivatedDate": true,
	"Microsoft.VSTS.Common.ActivatedBy": true, "Microsoft.VSTS.Common.ResolvedDate": true,
	"Microsoft.VSTS.Common.ResolvedBy": true, "Microsoft.VSTS.Common.ClosedDate": true, "Microsoft.VSTS.Common.ClosedBy": true,
}

// Test plans and suites are work items but can only be created through the
// Test Plan API, so the testplans component handles them.
var skipTypes = map[string]bool{"test plan": true, "test suite": true}

type relation struct {
	Rel        string         `json:"rel"`
	URL        string         `json:"url"`
	Attributes map[string]any `json:"attributes"`
}

type workItem struct {
	ID        int            `json:"id"`
	Fields    map[string]any `json:"fields"`
	Relations []relation     `json:"relations"`
}

const batchSize = 200 // workitemsbatch maximum

func fetchItems(ctx context.Context, x *Ctx, ids []int) ([]workItem, error) {
	var r struct {
		Value []*workItem `json:"value"`
	}
	body := jx.M{"ids": ids, "$expand": "Relations", "errorPolicy": "omit"}
	if err := x.C.Post(ctx, x.org("_apis/wit/workitemsbatch?api-version=7.1"), body, &r); err != nil {
		return nil, err
	}
	out := make([]workItem, 0, len(r.Value))
	for _, w := range r.Value {
		if w != nil {
			out = append(out, *w)
		}
	}
	return out, nil
}

func chunks(ids []int, n int) [][]int {
	var out [][]int
	for i := 0; i < len(ids); i += n {
		j := i + n
		if j > len(ids) {
			j = len(ids)
		}
		out = append(out, ids[i:j])
	}
	return out
}

// CloneWorkItems copies work items in two passes, 200 at a time so memory
// stays flat on big projects.
//
// Pass 1 creates each item at its current state (with bypassRules the
// original created/changed dates and people are kept) plus a hyperlink back
// to the source item.
// Pass 2 copies comments (paged), attachments (streamed; chunked above
// 128 MiB), and links. Work item links are added once, from the lower source
// ID, because the server adds the reverse link itself. Links to items in
// other projects of the org keep pointing at those items. Commit and branch
// links are re-pointed at the copied repos; pull request, build and wiki
// links keep pointing at the source, which stays as the archive. Shared step
// and shared parameter references in test cases are remapped.
func CloneWorkItems(ctx context.Context, x *Ctx) error {
	if _, err := x.SrcProject(ctx); err != nil {
		return err
	}
	if _, err := x.TgtProject(ctx); err != nil {
		return err
	}
	ids, err := x.C.WorkItemIDs(ctx, x.Src, "")
	if err != nil {
		return err
	}
	x.read(len(ids))
	inSource := make(map[int]bool, len(ids))
	for _, id := range ids {
		inSource[id] = true
	}
	q := "?suppressNotifications=true&api-version=7.1"
	if x.Opt.BypassRules {
		q = "?bypassRules=true&suppressNotifications=true&api-version=7.1"
	}

	skipped := map[int]bool{} // test plans and suites: the testplans component's job
	for _, chunk := range chunks(ids, batchSize) {
		items, err := fetchItems(ctx, x, chunk)
		if err != nil {
			return err
		}
		for _, w := range items {
			typ := jx.Str(w.Fields, "System.WorkItemType")
			if skipTypes[fold(typ)] {
				skipped[w.ID] = true
				continue
			}
			sid := itoa(w.ID)
			if _, done := x.mapped("workitem", sid); done {
				continue
			}
			var created struct {
				ID int `json:"id"`
			}
			u := x.tgt("_apis/wit/workitems/$" + client.P(typ) + q)
			if _, err := x.C.Do(ctx, http.MethodPost, u, "application/json-patch+json", x.createOps(w), &created); err != nil {
				x.fail("work item "+sid, err)
				continue
			}
			if x.Dry() {
				continue
			}
			x.wrote()
			x.record("workitem", sid, itoa(created.ID))
		}
	}
	if x.Dry() {
		return nil
	}

	gm := x.guidMap()
	for _, chunk := range chunks(ids, batchSize) {
		items, err := fetchItems(ctx, x, chunk)
		if err != nil {
			return err
		}
		for _, w := range items {
			sid := itoa(w.ID)
			tid, ok := x.mapped("workitem", sid)
			if !ok {
				continue // skipped type, or failed in pass 1
			}
			if done, _ := x.mapped("wicomments", sid); done != "1" {
				if err := x.copyComments(ctx, w.ID, atoi(tid)); err != nil {
					x.fail("comments of work item "+sid, err)
				} else {
					x.record("wicomments", sid, "1")
				}
			}
			if done, _ := x.mapped("wilinks", sid); done == "1" {
				continue
			}
			ops, waiting, err := x.linkOps(ctx, w, inSource, skipped, gm)
			if err != nil {
				x.fail("links of work item "+sid, err)
				continue
			}
			if len(ops) > 0 {
				if err := x.patchItem(ctx, tid, ops, q); err != nil {
					x.fail("links of work item "+sid, err)
					continue
				}
			}
			if len(waiting) > 0 {
				// Not marked done: the next run adds these links once the
				// other items exist (links already added are skipped as duplicates).
				x.todo("links of work item "+sid, "links to work items "+strings.Join(waiting, ", ")+" that weren't copied; fix those and rerun workitems")
				continue
			}
			x.record("wilinks", sid, "1")
		}
	}
	return nil
}

func (x *Ctx) createOps(w workItem) []any {
	keys := make([]string, 0, len(w.Fields))
	for k := range w.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var ops []any
	for _, k := range keys {
		v := w.Fields[k]
		if skipFields[k] || k == "System.WorkItemType" || strings.HasPrefix(k, "WEF_") {
			continue // WEF_ fields belong to the source team's board
		}
		if !x.Opt.BypassRules && ruleFields[k] {
			continue
		}
		switch k {
		case "System.AreaPath", "System.IterationPath":
			if s, ok := v.(string); ok {
				v = jx.RebasePath(s, x.Src, x.Tgt)
			}
		}
		if m, ok := v.(map[string]any); ok { // identity: same org, so the unique name resolves
			un, ok := m["uniqueName"].(string)
			if !ok {
				continue
			}
			v = x.rebaseGroupName(un)
		}
		ops = append(ops, jx.M{"op": "add", "path": "/fields/" + k, "value": v})
	}
	ops = append(ops, jx.M{"op": "add", "path": "/relations/-", "value": jx.M{
		"rel":        "Hyperlink",
		"url":        fmt.Sprintf("https://dev.azure.com/%s/%s/_workitems/edit/%d", x.C.Org, url.PathEscape(x.Src), w.ID),
		"attributes": jx.M{"comment": "Copied from " + x.Src},
	}})
	return ops
}

var wiURL = regexp.MustCompile(`(?i)/_apis/wit/workItems/(\d+)$`)

func (x *Ctx) linkOps(ctx context.Context, w workItem, inSource, skipped map[int]bool, gm map[string]string) ([]any, []string, error) {
	var ops []any
	var waiting []string
	addRel := func(rel, u string, attrs jx.M) {
		ops = append(ops, jx.M{"op": "add", "path": "/relations/-", "value": jx.M{"rel": rel, "url": u, "attributes": attrs}})
	}
	comment := func(r relation) jx.M { return jx.M{"comment": r.Attributes["comment"]} }
	for _, r := range w.Relations {
		switch {
		case r.Rel == "AttachedFile":
			nu, err := x.copyAttachment(ctx, r.URL, fmt.Sprint(r.Attributes["name"]))
			if err != nil {
				return nil, nil, fmt.Errorf("attachment %v: %w", r.Attributes["name"], err)
			}
			addRel(r.Rel, nu, comment(r))
		case r.Rel == "ArtifactLink":
			attrs := comment(r)
			attrs["name"] = r.Attributes["name"]
			addRel(r.Rel, remapArtifact(r.URL, gm), attrs)
		case strings.Contains(r.Rel, ".Remote."): // cross-organization links
			addRel(r.Rel, r.URL, comment(r))
		case wiURL.MatchString(r.URL):
			other, _ := strconv.Atoi(wiURL.FindStringSubmatch(r.URL)[1])
			if !inSource[other] { // another project in the org: keep pointing at it
				addRel(r.Rel, x.org("_apis/wit/workItems/"+itoa(other)), comment(r))
				continue
			}
			if other < w.ID {
				continue // added from the other end, which has the lower ID
			}
			t, ok := x.mapped("workitem", itoa(other))
			if !ok {
				if !skipped[other] {
					waiting = append(waiting, itoa(other))
				}
				continue
			}
			addRel(r.Rel, x.org("_apis/wit/workItems/"+t), comment(r))
		default: // hyperlinks and anything else
			addRel(r.Rel, r.URL, comment(r))
		}
	}
	if s := jx.Str(w.Fields, "Microsoft.VSTS.TCM.Steps"); strings.Contains(s, "compref") {
		if ns := x.remapSharedSteps(s); ns != s {
			ops = append(ops, jx.M{"op": "add", "path": "/fields/Microsoft.VSTS.TCM.Steps", "value": ns})
		}
	}
	if s := jx.Str(w.Fields, "Microsoft.VSTS.TCM.LocalDataSource"); strings.Contains(s, "sharedParameterDataSetIds") {
		if ns := x.remapSharedParams(s); ns != s {
			ops = append(ops, jx.M{"op": "add", "path": "/fields/Microsoft.VSTS.TCM.LocalDataSource", "value": ns})
		}
	}
	return ops, waiting, nil
}

var gitArtifact = regexp.MustCompile(`(?i)^(vstfs:///Git/(?:Commit|Ref)/)([0-9a-f-]{36})%2f([0-9a-f-]{36})%2f(.*)$`)

// remapArtifact re-points commit and branch links at the copied repo (same
// SHAs, same branches). Everything else is returned unchanged.
func remapArtifact(u string, gm map[string]string) string {
	m := gitArtifact.FindStringSubmatch(u)
	if m == nil {
		return u
	}
	proj, okP := gm[strings.ToLower(m[2])]
	repo, okR := gm[strings.ToLower(m[3])]
	if !okP || !okR {
		return u
	}
	return m[1] + proj + "%2F" + repo + "%2F" + m[4]
}

var comprefRE = regexp.MustCompile(`(<compref\b[^>]*\bref=")(\d+)(")`)

func (x *Ctx) remapSharedSteps(s string) string {
	return comprefRE.ReplaceAllStringFunc(s, func(m string) string {
		p := comprefRE.FindStringSubmatch(m)
		if t, ok := x.mapped("workitem", p[2]); ok {
			return p[1] + t + p[3]
		}
		return m
	})
}

var sharedParamsRE = regexp.MustCompile(`("sharedParameterDataSetIds"\s*:\s*\[)([^\]]*)(\])`)

func (x *Ctx) remapSharedParams(s string) string {
	return sharedParamsRE.ReplaceAllStringFunc(s, func(m string) string {
		p := sharedParamsRE.FindStringSubmatch(m)
		ids := strings.Split(p[2], ",")
		for i, id := range ids {
			if t, ok := x.mapped("workitem", strings.TrimSpace(id)); ok {
				ids[i] = t
			}
		}
		return p[1] + strings.Join(ids, ",") + p[3]
	})
}

// patchItem applies ops in one request. If the server rejects the batch (one
// link that already exists fails the lot), it applies them one at a time and
// skips the duplicates.
func (x *Ctx) patchItem(ctx context.Context, tid string, ops []any, q string) error {
	u := x.org("_apis/wit/workitems/" + tid + q)
	const ct = "application/json-patch+json"
	_, err := x.C.Do(ctx, http.MethodPatch, u, ct, ops, nil)
	if err == nil || client.StatusOf(err) != http.StatusBadRequest {
		return err
	}
	var errs []string
	for _, op := range ops {
		if _, e := x.C.Do(ctx, http.MethodPatch, u, ct, []any{op}, nil); e != nil && !isDuplicateLink(e) {
			errs = append(errs, e.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func isDuplicateLink(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "tf201035") || strings.Contains(s, "already exist")
}

// singleUploadMax is the largest attachment sent in one request; bigger ones
// use the chunked upload API. Variables so tests can shrink them.
var (
	singleUploadMax int64 = 128 << 20
	chunkSize             = 16 << 20
)

// copyAttachment streams the source attachment to a temp file, then uploads
// it to the target in one request or in chunks.
func (x *Ctx) copyAttachment(ctx context.Context, srcURL, name string) (string, error) {
	key := strings.ToLower(srcURL[strings.LastIndex(srcURL, "/")+1:])
	if u, ok := x.mapped("attachment", key); ok {
		return u, nil // uploaded by an earlier run whose link update didn't finish
	}
	u, err := x.uploadAttachment(ctx, srcURL, name)
	if err == nil {
		x.record("attachment", key, u)
	}
	return u, err
}

func (x *Ctx) uploadAttachment(ctx context.Context, srcURL, name string) (string, error) {
	f, err := os.CreateTemp(x.Opt.WorkDir, "adoclone-att-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	sep := "?"
	if strings.Contains(srcURL, "?") {
		sep = "&"
	}
	n, err := x.C.Download(ctx, srcURL+sep+"download=true&api-version=7.1", f)
	if err != nil {
		return "", err
	}
	base := x.tgt("_apis/wit/attachments")
	fn := "fileName=" + url.QueryEscape(name)
	var r struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if n <= singleUploadMax {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		data, err := io.ReadAll(f)
		if err != nil {
			return "", err
		}
		_, err = x.C.Do(ctx, http.MethodPost, base+"?"+fn+"&api-version=7.1", "application/octet-stream", data, &r)
		return r.URL, err
	}
	if _, err := x.C.Do(ctx, http.MethodPost, base+"?"+fn+"&uploadType=Chunked&api-version=7.1", "application/octet-stream", []byte{}, &r); err != nil {
		return "", err
	}
	buf := make([]byte, chunkSize)
	for off := int64(0); off < n; {
		k, err := f.ReadAt(buf, off)
		if k == 0 {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return "", err
		}
		rng := fmt.Sprintf("bytes %d-%d/%d", off, off+int64(k)-1, n)
		if _, err := x.C.Do(ctx, http.MethodPut, base+"/"+r.ID+"?"+fn+"&api-version=7.1", "application/octet-stream",
			buf[:k], nil, client.WithHeader("Content-Range", rng)); err != nil {
			return "", fmt.Errorf("chunk %s: %w", rng, err)
		}
		off += int64(k)
	}
	return r.URL, nil
}

// copyComments copies every comment, oldest first, following continuation
// tokens. The API only takes text, so the original author and date go in
// front. Each copied comment is recorded, so a rerun after a failure doesn't
// post duplicates.
func (x *Ctx) copyComments(ctx context.Context, src, tgt int) error {
	token, n := "", 0
	for {
		u := x.src(fmt.Sprintf("_apis/wit/workItems/%d/comments?order=asc&$top=200&api-version=7.1-preview.4", src))
		if token != "" {
			u += "&continuationToken=" + url.QueryEscape(token)
		}
		var r struct {
			Comments []struct {
				ID          int    `json:"id"`
				CommentID   int    `json:"commentId"`
				Text        string `json:"text"`
				CreatedDate string `json:"createdDate"`
				CreatedBy   struct {
					DisplayName string `json:"displayName"`
				} `json:"createdBy"`
			} `json:"comments"`
			ContinuationToken string `json:"continuationToken"`
		}
		if err := x.C.Get(ctx, u, &r); err != nil {
			return err
		}
		for _, c := range r.Comments {
			n++
			key := fmt.Sprintf("%d:#%d", src, n)
			id := c.ID
			if id == 0 {
				id = c.CommentID
			}
			if id != 0 {
				key = fmt.Sprintf("%d:%d", src, id)
			}
			if _, done := x.mapped("wicomment", key); done {
				continue
			}
			txt := fmt.Sprintf("<p><i>[copied] %s — %s</i></p>%s", c.CreatedBy.DisplayName, c.CreatedDate, c.Text)
			pu := x.tgt(fmt.Sprintf("_apis/wit/workItems/%d/comments?api-version=7.1-preview.4", tgt))
			if err := x.C.Post(ctx, pu, jx.M{"text": txt}, nil); err != nil {
				return err
			}
			x.record("wicomment", key, "1")
		}
		if r.ContinuationToken == "" || len(r.Comments) == 0 || r.ContinuationToken == token {
			return nil
		}
		token = r.ContinuationToken
	}
}
