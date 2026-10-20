// Package ledger is the library card file: a JSON array of loans kept in
// git (ledger/ledger.json). Each loan is appended once and later closed by
// setting returned_at; nothing is ever deleted, so the file is its own
// history and git blame is the audit log.
//
// Rules, enforced on Borrow:
//
//   - the item must exist (the caller says so; the ledger does not know items);
//   - an item has at most one open loan;
//   - a login has at most MaxOpen open loans;
//   - loans are 7 or 14 days; due = borrowed_at + days.
//
// Overdue loans are warned once they pass due, and returned automatically
// once they pass due + grace.
package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxOpen is how many items one login may hold at once.
const MaxOpen = 3

// Loan is one ledger record.
type Loan struct {
	Item        string     `json:"item"`
	Borrower    string     `json:"borrower"`
	BorrowIssue int        `json:"borrow_issue"`
	BorrowedAt  time.Time  `json:"borrowed_at"`
	Due         time.Time  `json:"due"`
	ReturnedAt  *time.Time `json:"returned_at"`
	ReturnedBy  string     `json:"returned_by,omitempty"` // login, or "overdue" for auto-returns
	Reason      string     `json:"reason,omitempty"`
}

// Open reports whether the loan is outstanding.
func (l *Loan) Open() bool { return l.ReturnedAt == nil }

// Ledger is the whole file.
type Ledger struct {
	Loans []*Loan
}

// Errors callers match on to choose a comment.
var (
	ErrNoItem       = errors.New("ledger: no such item")
	ErrBorrowed     = errors.New("ledger: item is already borrowed")
	ErrLimit        = fmt.Errorf("ledger: borrower already holds %d items", MaxOpen)
	ErrNotBorrowed  = errors.New("ledger: item is not borrowed")
	ErrNotBorrower  = errors.New("ledger: only the borrower may return an item")
	ErrBadDays      = errors.New("ledger: days must be 7 or 14")
	ErrBadBorrower  = errors.New("ledger: borrower login is empty")
	ErrBadIssueNumb = errors.New("ledger: borrow issue must be positive")
)

// Load reads path; a missing file is an empty ledger.
func Load(path string) (*Ledger, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Ledger{}, nil
	}
	if err != nil {
		return nil, err
	}
	l := &Ledger{}
	if len(strings.TrimSpace(string(b))) == 0 {
		return l, nil
	}
	if err := json.Unmarshal(b, &l.Loans); err != nil {
		return nil, fmt.Errorf("ledger: %s: %w", path, err)
	}
	return l, nil
}

// Marshal returns the file contents: one loan per element, indented,
// trailing newline, so diffs are line-per-field and merge cleanly.
func (l *Ledger) Marshal() ([]byte, error) {
	loans := l.Loans
	if loans == nil {
		loans = []*Loan{}
	}
	b, err := json.MarshalIndent(loans, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Save writes the ledger atomically (temp file + rename).
func (l *Ledger) Save(path string) error {
	b, err := l.Marshal()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ledger-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// OpenLoan returns the open loan for item, or nil.
func (l *Ledger) OpenLoan(item string) *Loan {
	for _, x := range l.Loans {
		if x.Item == item && x.Open() {
			return x
		}
	}
	return nil
}

// OpenBy returns the open loans held by login (case-insensitive: GitHub
// logins are).
func (l *Ledger) OpenBy(login string) []*Loan {
	var out []*Loan
	for _, x := range l.Loans {
		if x.Open() && strings.EqualFold(x.Borrower, login) {
			out = append(out, x)
		}
	}
	return out
}

// Borrow records a new loan. exists says whether the item is in the
// library; the ledger keeps no catalog of its own.
func (l *Ledger) Borrow(item, by string, days, issue int, reason string, exists bool, now time.Time) (*Loan, error) {
	switch {
	case !exists:
		return nil, fmt.Errorf("%w: %s", ErrNoItem, item)
	case by == "":
		return nil, ErrBadBorrower
	case days != 7 && days != 14:
		return nil, ErrBadDays
	case issue <= 0:
		return nil, ErrBadIssueNumb
	}
	if cur := l.OpenLoan(item); cur != nil {
		// The same borrow issue arriving twice (an edited issue is
		// re-dispatched) is the same loan, not a conflict.
		if cur.BorrowIssue == issue && strings.EqualFold(cur.Borrower, by) {
			return cur, nil
		}
		return nil, fmt.Errorf("%w: %s by %s until %s", ErrBorrowed, item, cur.Borrower, cur.Due.Format("2006-01-02"))
	}
	if n := len(l.OpenBy(by)); n >= MaxOpen {
		return nil, fmt.Errorf("%w (%s has %d)", ErrLimit, by, n)
	}
	now = now.UTC().Truncate(time.Second)
	loan := &Loan{Item: item, Borrower: by, BorrowIssue: issue, BorrowedAt: now,
		Due: now.Add(time.Duration(days) * 24 * time.Hour), Reason: reason}
	l.Loans = append(l.Loans, loan)
	return loan, nil
}

// Return closes the open loan on item. by must be the borrower unless
// force is set (operators and the overdue sweep).
func (l *Ledger) Return(item, by string, force bool, now time.Time) (*Loan, error) {
	loan := l.OpenLoan(item)
	if loan == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotBorrowed, item)
	}
	if !force && !strings.EqualFold(loan.Borrower, by) {
		return nil, fmt.Errorf("%w (%s holds %s)", ErrNotBorrower, loan.Borrower, item)
	}
	t := now.UTC().Truncate(time.Second)
	loan.ReturnedAt = &t
	loan.ReturnedBy = by
	return loan, nil
}

// Notice is one overdue finding.
type Notice struct {
	Loan     *Loan  `json:"loan"`
	Action   string `json:"action"` // "warn" or "return"
	DaysLate int    `json:"days_late"`
}

// Overdue finds open loans past due. Loans past due+grace are returned
// (ReturnedBy "overdue"); the rest are warnings. The ledger is modified
// only when apply is set, so a dry run can print the plan.
func (l *Ledger) Overdue(now time.Time, grace time.Duration, apply bool) []Notice {
	var out []Notice
	now = now.UTC().Truncate(time.Second)
	for _, x := range l.Loans {
		if !x.Open() || !now.After(x.Due) {
			continue
		}
		n := Notice{Loan: x, Action: "warn", DaysLate: int(now.Sub(x.Due) / (24 * time.Hour))}
		if now.After(x.Due.Add(grace)) {
			n.Action = "return"
			if apply {
				t := now
				x.ReturnedAt = &t
				x.ReturnedBy = "overdue"
			}
		}
		out = append(out, n)
	}
	return out
}

// OpenLoans returns open loans sorted by due date, then item.
func (l *Ledger) OpenLoans() []*Loan {
	var out []*Loan
	for _, x := range l.Loans {
		if x.Open() {
			out = append(out, x)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Due.Equal(out[j].Due) {
			return out[i].Due.Before(out[j].Due)
		}
		return out[i].Item < out[j].Item
	})
	return out
}

// History returns every loan, newest borrow first.
func (l *Ledger) History() []*Loan {
	out := append([]*Loan(nil), l.Loans...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].BorrowedAt.After(out[j].BorrowedAt) })
	return out
}
