package clone

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/coolado/adoclone/internal/jx"
)

// CloneHooks copies service hook subscriptions on the source project.
// Consumer secrets come back masked, so subscriptions that had one are created
// disabled and listed in todo.json for the secret to be re-entered.
func CloneHooks(ctx context.Context, x *Ctx) error {
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	all, err := valueOf[jx.M](ctx, x, x.org("_apis/hooks/subscriptions?api-version=7.1"))
	if err != nil {
		return err
	}
	have := map[string]bool{}
	var subs []jx.M
	for _, s := range all {
		switch pid := jx.Str(s, "publisherInputs", "projectId"); {
		case strings.EqualFold(pid, src.ID):
			subs = append(subs, s)
		case tgt.ID != "" && strings.EqualFold(pid, tgt.ID):
			have[hookSig(s)] = true
		}
	}
	x.read(len(subs))
	gm := x.guidMap()
	for _, s := range subs {
		sid := jx.Str(s, "id")
		if _, done := x.mapped("hook", sid); done {
			continue
		}
		body := x.transformHook(s, gm)
		if have[hookSig(body)] {
			continue
		}
		item := "service hook " + jx.Str(s, "eventType") + " -> " + jx.Str(s, "consumerId")
		var created jx.M
		if err := x.C.Post(ctx, x.org("_apis/hooks/subscriptions?api-version=7.1"), body, &created); err != nil {
			x.fail(item, err)
			continue
		}
		x.wrote()
		if x.Dry() {
			continue
		}
		x.record("hook", sid, jx.Str(created, "id"))
		if hasMaskedSecret(jx.Obj(s, "consumerInputs")) {
			created["status"] = "disabledByUser"
			if err := x.C.Put(ctx, x.org("_apis/hooks/subscriptions/"+jx.Str(created, "id")+"?api-version=7.1"), created, nil); err != nil {
				x.fail(item+" (disable)", err)
			}
			x.todo(item, "re-enter the masked secret in the target subscription, then enable it")
		}
	}
	return nil
}

func (x *Ctx) transformHook(s jx.M, gm map[string]string) jx.M {
	pin := jx.Copy(jx.Obj(s, "publisherInputs"))
	delete(pin, "tfsSubscriptionId")
	bd, rd, re := x.S.Map("buildDef"), x.S.Map("releaseDef"), x.S.Map("releaseEnv")
	for k, v := range pin {
		str, _ := v.(string)
		switch fold(k) {
		case "areapath":
			pin[k] = jx.RebasePath(str, x.Src, x.Tgt)
		case "releasedefinitionid":
			if t, ok := rd[str]; ok {
				pin[k] = t
			}
		case "releaseenvironmentid":
			if t, ok := re[str]; ok {
				pin[k] = t
			}
		case "pipelineid", "definitionid", "builddefinitionid":
			if t, ok := bd[str]; ok {
				pin[k] = t
			}
		}
	}
	body := jx.M{"publisherInputs": jx.RemapJSON(pin, gm)}
	for _, k := range []string{"publisherId", "eventType", "resourceVersion", "consumerId", "consumerActionId", "consumerInputs"} {
		if v, ok := s[k]; ok {
			body[k] = v
		}
	}
	return body
}

func hookSig(s jx.M) string {
	pin := jx.Copy(jx.Obj(s, "publisherInputs"))
	delete(pin, "tfsSubscriptionId")
	delete(pin, "projectId")
	b, _ := json.Marshal(pin)
	return jx.Str(s, "eventType") + "|" + jx.Str(s, "consumerId") + "|" + jx.Str(s, "consumerActionId") + "|" +
		jx.Str(s, "consumerInputs", "url") + "|" + string(b)
}

func hasMaskedSecret(in jx.M) bool {
	for _, v := range in {
		if s, ok := v.(string); ok && strings.Contains(s, "********") {
			return true
		}
	}
	return false
}
