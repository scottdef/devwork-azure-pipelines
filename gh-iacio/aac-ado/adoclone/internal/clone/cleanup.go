package clone

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coolado/adoclone/internal/client"
)

// Cleanup removes what adoclone shared with the target (service connections
// and, in share mode, variable groups it shared itself), deletes the target
// project (soft delete, restorable for the retention period), and moves the
// checkpoint aside. It refuses to delete the source or a target that doesn't
// match the checkpoint.
func Cleanup(ctx context.Context, x *Ctx) error {
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	tp, err := getProject(ctx, x.C, x.Tgt)
	if client.IsNotFound(err) {
		x.Log.Info("target project doesn't exist; nothing to delete")
		return nil
	}
	if err != nil {
		return err
	}
	if strings.EqualFold(tp.ID, src.ID) {
		return fmt.Errorf("refusing to delete: target %q is the source project", x.Tgt)
	}
	if x.S.TargetProjectID == "" && !x.Opt.AdoptExisting {
		return fmt.Errorf("refusing to delete: the checkpoint doesn't know target project %q; run cleanup with the -state file from the clone, or pass -adopt-existing", x.Tgt)
	}
	if x.S.TargetProjectID != "" && !strings.EqualFold(x.S.TargetProjectID, tp.ID) {
		return fmt.Errorf("refusing to delete: the checkpoint belongs to target project %s, not %s", x.S.TargetProjectID, tp.ID)
	}
	for id := range x.S.Map("endpointShared") {
		if err := x.C.Delete(ctx, x.org("_apis/serviceendpoint/endpoints/"+id+"?projectIds="+tp.ID+"&api-version=7.1"), nil); err != nil {
			x.fail("unshare service connection "+id, err)
		}
	}
	for id := range x.S.Map("vargroupShared") {
		if err := x.C.Delete(ctx, x.org("_apis/distributedtask/variablegroups/"+id+"?projectIds="+tp.ID+"&api-version=7.1"), nil); err != nil {
			x.fail("unshare variable group "+id, err)
		}
	}
	var op struct {
		ID string `json:"id"`
	}
	if err := x.C.Delete(ctx, x.org("_apis/projects/"+tp.ID+"?api-version=7.1"), &op); err != nil {
		return err
	}
	if x.Dry() {
		return nil
	}
	if err := waitOperation(ctx, x, op.ID); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	x.Log.Info("target project deleted (restorable from Organization settings > Projects during the retention period)", "id", tp.ID)
	return x.S.Rename(".deleted-" + time.Now().UTC().Format("20060102T150405Z"))
}
