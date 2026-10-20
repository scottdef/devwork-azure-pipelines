package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoolGitOrg/issueops-library/internal/golden"
)

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func TestBorrowLimit(t *testing.T) {
	l := &Ledger{}
	for i, it := range []string{"a1/readme/1", "a1/readme/2", "a1/readme/3"} {
		if _, err := l.Borrow(it, "bob", 7, 10+i, "", true, t0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.Borrow("a1/readme/4", "BOB", 7, 20, "", true, t0); !errors.Is(err, ErrLimit) {
		t.Errorf("4th borrow (case-folded login): err = %v", err)
	}
	if _, err := l.Borrow("a1/readme/4", "carol", 7, 21, "", true, t0); err != nil {
		t.Errorf("other login blocked: %v", err)
	}
	if _, err := l.Return("a1/readme/1", "bob", false, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Borrow("a1/readme/5", "bob", 14, 22, "", true, t0); err != nil {
		t.Errorf("after return, limit should free up: %v", err)
	}
}

func TestDoubleBorrow(t *testing.T) {
	l := &Ledger{}
	if _, err := l.Borrow("a1/readme/1", "bob", 7, 1, "", true, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Borrow("a1/readme/1", "carol", 7, 2, "", true, t0); !errors.Is(err, ErrBorrowed) {
		t.Errorf("err = %v", err)
	}
	if _, err := l.Borrow("a1/readme/1", "bob", 7, 3, "", true, t0); !errors.Is(err, ErrBorrowed) {
		t.Errorf("same borrower, new issue: err = %v", err)
	}
	// Re-dispatch of the same borrow issue (edited issue) is a no-op.
	if loan, err := l.Borrow("a1/readme/1", "bob", 7, 1, "", true, t0.Add(time.Hour)); err != nil || !loan.BorrowedAt.Equal(t0) || len(l.Loans) != 1 {
		t.Errorf("replayed borrow: %v %+v (%d loans)", err, loan, len(l.Loans))
	}
}

func TestBorrowValidation(t *testing.T) {
	l := &Ledger{}
	for name, c := range map[string]struct {
		by     string
		days   int
		issue  int
		exists bool
		want   error
	}{
		"noitem": {"bob", 7, 1, false, ErrNoItem},
		"days":   {"bob", 30, 1, true, ErrBadDays},
		"login":  {"", 7, 1, true, ErrBadBorrower},
		"issue":  {"bob", 7, 0, true, ErrBadIssueNumb},
	} {
		if _, err := l.Borrow("a1/readme/1", c.by, c.days, c.issue, "", c.exists, t0); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if len(l.Loans) != 0 {
		t.Error("refused borrows were recorded")
	}
}

func TestReturn(t *testing.T) {
	l := &Ledger{}
	loan, _ := l.Borrow("a1/readme/1", "bob", 14, 1, "", true, t0)
	if want := t0.Add(14 * 24 * time.Hour); !loan.Due.Equal(want) {
		t.Errorf("due = %v, want %v", loan.Due, want)
	}
	if _, err := l.Return("a1/readme/1", "carol", false, t0); !errors.Is(err, ErrNotBorrower) {
		t.Errorf("stranger return: %v", err)
	}
	if _, err := l.Return("a1/readme/1", "carol", true, t0); err != nil {
		t.Errorf("forced return: %v", err)
	}
	if _, err := l.Return("a1/readme/1", "bob", false, t0); !errors.Is(err, ErrNotBorrowed) {
		t.Errorf("double return: %v", err)
	}
	if l.OpenLoan("a1/readme/1") != nil || len(l.History()) != 1 {
		t.Error("state after return wrong")
	}
}

func TestOverdue(t *testing.T) {
	l := &Ledger{}
	l.Borrow("a1/readme/1", "bob", 7, 1, "", true, t0)                        // due t0+7d
	l.Borrow("a1/readme/2", "bob", 14, 2, "", true, t0)                       // due t0+14d
	l.Borrow("a1/readme/3", "carol", 7, 3, "", true, t0.Add(-7*24*time.Hour)) // due t0
	now := t0.Add(15 * 24 * time.Hour)
	grace := 7 * 24 * time.Hour

	dry := l.Overdue(now, grace, false)
	if len(dry) != 3 || l.OpenLoan("a1/readme/3") == nil {
		t.Fatalf("dry run changed state or missed loans: %+v", dry)
	}
	got := map[string]string{}
	for _, n := range l.Overdue(now, grace, true) {
		got[n.Loan.Item] = n.Action
	}
	want := map[string]string{"a1/readme/1": "return", "a1/readme/2": "warn", "a1/readme/3": "return"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %s, want %s", k, got[k], v)
		}
	}
	if l.OpenLoan("a1/readme/1") != nil || l.OpenLoan("a1/readme/2") == nil {
		t.Error("apply did not return the right loans")
	}
	if x := l.Loans[0]; x.ReturnedBy != "overdue" {
		t.Errorf("returned_by = %q", x.ReturnedBy)
	}
	if n := l.Overdue(t0, grace, true); len(n) != 0 {
		t.Errorf("nothing is overdue at t0: %+v", n)
	}
}

func TestSaveLoadGolden(t *testing.T) {
	l := &Ledger{}
	l.Borrow("payments-api/caddyfile/42", "bob", 14, 50, "phase 2 rollout", true, t0)
	l.Borrow("infra-net/terraform-module/45", "carol", 7, 55, "", true, t0)
	l.Return("infra-net/terraform-module/45", "carol", false, t0.Add(48*time.Hour))
	p := filepath.Join(t.TempDir(), "ledger", "ledger.json")
	if err := l.Save(p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	golden.Check(t, "../../testdata/golden/ledger/ledger.json", b)
	back, err := Load(p)
	if err != nil || len(back.Loans) != 2 || back.OpenLoan("payments-api/caddyfile/42") == nil {
		t.Errorf("reload: %v %+v", err, back)
	}
	if e, err := Load(filepath.Join(t.TempDir(), "none.json")); err != nil || len(e.Loans) != 0 {
		t.Error("missing file should be an empty ledger")
	}
	empty := &Ledger{}
	if b, _ := empty.Marshal(); string(b) != "[]\n" {
		t.Errorf("empty marshal = %q", b)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte("{"), 0o644)
	if _, err := Load(bad); err == nil {
		t.Error("corrupt ledger: want error")
	}
}
