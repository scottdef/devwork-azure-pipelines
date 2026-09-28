package ghapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// BudgetAlerting configures budget alerts.
type BudgetAlerting struct {
	WillAlert       bool     `json:"will_alert"`
	AlertRecipients []string `json:"alert_recipients"`
}

// Budget as returned by the budgets API. The list endpoint returns
// budget_product_skus (array) while get/create return budget_product_sku.
type Budget struct {
	ID                  string         `json:"id"`
	BudgetType          string         `json:"budget_type"`
	BudgetProductSKU    string         `json:"budget_product_sku,omitempty"`
	BudgetProductSKUs   []string       `json:"budget_product_skus,omitempty"`
	BudgetScope         string         `json:"budget_scope"`
	BudgetEntityName    string         `json:"budget_entity_name,omitempty"`
	BudgetAmount        float64        `json:"budget_amount"`
	PreventFurtherUsage bool           `json:"prevent_further_usage"`
	BudgetAlerting      BudgetAlerting `json:"budget_alerting"`
	User                string         `json:"user,omitempty"`
	ExpiresAt           string         `json:"expires_at,omitempty"`
}

// SKUs returns the product SKUs covered by the budget.
func (b *Budget) SKUs() []string {
	if len(b.BudgetProductSKUs) > 0 {
		return b.BudgetProductSKUs
	}
	if b.BudgetProductSKU != "" {
		return []string{b.BudgetProductSKU}
	}
	return nil
}

// BudgetCreate is the create payload.
type BudgetCreate struct {
	BudgetAmount        int            `json:"budget_amount"`
	PreventFurtherUsage bool           `json:"prevent_further_usage"`
	BudgetAlerting      BudgetAlerting `json:"budget_alerting"`
	BudgetScope         string         `json:"budget_scope"`
	BudgetEntityName    string         `json:"budget_entity_name"`
	BudgetType          string         `json:"budget_type"`
	BudgetProductSKU    string         `json:"budget_product_sku"`
	User                string         `json:"user,omitempty"`
	ExpiresAt           string         `json:"expires_at,omitempty"`
}

// BudgetUpdate is the update payload.
type BudgetUpdate struct {
	BudgetAmount        int             `json:"budget_amount"`
	PreventFurtherUsage bool            `json:"prevent_further_usage"`
	BudgetAlerting      *BudgetAlerting `json:"budget_alerting,omitempty"`
	ExpiresAt           string          `json:"expires_at,omitempty"`
}

// BudgetOwner selects the organization or enterprise budgets endpoint.
type BudgetOwner struct {
	Enterprise bool
	Name       string
}

func (o BudgetOwner) base() string {
	if o.Enterprise {
		return fmt.Sprintf("enterprises/%s/settings/billing/budgets", o.Name)
	}
	return fmt.Sprintf("organizations/%s/settings/billing/budgets", o.Name)
}

// ListBudgets lists budgets, optionally filtered by scope and user.
func (c *Client) ListBudgets(ctx context.Context, owner BudgetOwner, scope, user string) ([]Budget, error) {
	var all []Budget
	for page := 1; page <= 50; page++ {
		q := url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}}
		if scope != "" {
			q.Set("scope", scope)
		}
		if user != "" {
			q.Set("user", user)
		}
		var out struct {
			Budgets     []Budget `json:"budgets"`
			HasNextPage bool     `json:"has_next_page"`
		}
		if _, err := c.JSON(ctx, Request{Path: owner.base(), Query: q, Version: VersionLatest}, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Budgets...)
		if !out.HasNextPage {
			break
		}
	}
	return all, nil
}

// CreateBudget creates a budget.
func (c *Client) CreateBudget(ctx context.Context, owner BudgetOwner, in BudgetCreate) (*Budget, error) {
	var out struct {
		Message string `json:"message"`
		Budget  Budget `json:"budget"`
	}
	_, err := c.JSON(ctx, Request{Method: http.MethodPost, Path: owner.base(), Body: in, Version: VersionLatest}, &out)
	return &out.Budget, err
}

// UpdateBudget updates a budget.
func (c *Client) UpdateBudget(ctx context.Context, owner BudgetOwner, id string, in BudgetUpdate) (*Budget, error) {
	var out struct {
		Message string `json:"message"`
		Budget  Budget `json:"budget"`
	}
	_, err := c.JSON(ctx, Request{Method: http.MethodPatch, Path: owner.base() + "/" + url.PathEscape(id), Body: in, Version: VersionLatest}, &out)
	return &out.Budget, err
}

// UsageItem is one line of a billing usage report.
type UsageItem struct {
	Date           string  `json:"date,omitempty"`
	Product        string  `json:"product"`
	SKU            string  `json:"sku"`
	Model          string  `json:"model,omitempty"`
	UnitType       string  `json:"unitType"`
	PricePerUnit   float64 `json:"pricePerUnit"`
	GrossQuantity  float64 `json:"grossQuantity"`
	GrossAmount    float64 `json:"grossAmount"`
	DiscountAmount float64 `json:"discountAmount"`
	NetQuantity    float64 `json:"netQuantity"`
	NetAmount      float64 `json:"netAmount"`
}

// AICreditUsage returns the organization's AI-credit usage for a month
// (optionally one user). Only the last 24 months are available.
func (c *Client) AICreditUsage(ctx context.Context, org string, year, month int, user string) ([]UsageItem, error) {
	q := url.Values{"year": {strconv.Itoa(year)}, "month": {strconv.Itoa(month)}}
	if user != "" {
		q.Set("user", user)
	}
	var out struct {
		UsageItems []UsageItem `json:"usageItems"`
	}
	_, err := c.JSON(ctx, Request{Path: fmt.Sprintf("organizations/%s/settings/billing/ai_credit/usage", org), Query: q, Version: VersionLatest}, &out)
	return out.UsageItems, err
}

// MarshalIndent is a helper for rendering payloads into comments and logs.
func MarshalIndent(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
