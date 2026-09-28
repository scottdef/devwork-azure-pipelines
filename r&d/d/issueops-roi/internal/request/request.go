// Package request defines the contract every request-type handler implements
// (Copilot budgets, Copilot reports, Foundry deployments, agentic tasks).
package request

import (
	"context"
	"time"

	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/ghapi"
	"github.com/CoolEngOrg/issueops/internal/issueform"
)

// Context is the input to a handler.
type Context struct {
	Registry  *config.Registry
	Type      *config.RequestType
	Values    issueform.Values
	Requestor string
	Issue     int
	// GH is nil in offline mode (local runs, tests, examples); handlers must
	// then skip live lookups.
	GH  *ghapi.Client
	Now time.Time
	// Info collects handler-specific details for comment templates
	// (current budget, resolved Foundry account, question status, ...).
	Info map[string]any
}

// SetInfo records a template detail.
func (c *Context) SetInfo(k string, v any) {
	if c.Info == nil {
		c.Info = map[string]any{}
	}
	c.Info[k] = v
}

// Row is one line of the request summary table.
type Row struct {
	Label string
	Value string
	Code  bool
}

// Handler implements type-specific policy.
type Handler interface {
	// Facts derives "$"-prefixed policy facts used by conditions and
	// environment placeholders.
	Facts(c *Context) (map[string]any, error)
	// Validate returns blocking errors and non-blocking warnings.
	Validate(ctx context.Context, c *Context) (errs, warns []string)
	// Summary describes the request for humans.
	Summary(c *Context) []Row
	// Subject returns the target repository (for target_repo_permissions) and
	// form fields that name users (for field_users selectors).
	Subject(c *Context) (targetRepo string, fieldUsers map[string]string)
}
