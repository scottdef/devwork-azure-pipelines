package site

import (
	"os/exec"
	"testing"

	"github.com/CoolGitOrg/issueops-library/internal/golden"
)

// TestLibraryJS runs the catalog filter (static/library.js) under node
// against the golden catalog page. Skipped without node.
func TestLibraryJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "../../testdata/js/library_test.js", "static/library.js", "../../testdata/golden/site/index.html").CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	golden.Check(t, "../../testdata/golden/js/library-filter.txt", out)
}
