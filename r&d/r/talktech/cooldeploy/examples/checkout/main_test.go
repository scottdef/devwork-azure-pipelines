package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T, cfg config) (*app, *httptest.Server) {
	t.Helper()
	if cfg.Currency == "" {
		cfg.Currency = "USD"
	}
	if cfg.Env == "" {
		cfg.Env = "local"
	}
	cfg.Release = "rtest001"
	a, err := newApp(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.routes())
	t.Cleanup(srv.Close)
	return a, srv
}

func body(t *testing.T, resp *http.Response, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestSampleData(t *testing.T) {
	s, err := newShop(files)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s.Products("")); n != 12 {
		t.Errorf("products = %d, want 12", n)
	}
	if n := len(s.Products("desk")); n != 3 {
		t.Errorf("search for desk = %d products, want 3", n)
	}
	// Every seeded order must add up: the sample data is training material.
	for _, o := range s.Orders(100) {
		sum := 0
		for _, it := range o.Items {
			p, err := s.Product(it.SKU)
			if err != nil || p.Name != it.Name || p.PriceCents != it.UnitCents {
				t.Errorf("order %s: item %s does not match the catalogue", o.ID, it.SKU)
			}
			sum += it.Qty * it.UnitCents
		}
		if sum != o.TotalCents {
			t.Errorf("order %s: items add up to %d, total says %d", o.ID, sum, o.TotalCents)
		}
	}
}

func TestOrderLifecycle(t *testing.T) {
	s, _ := newShop(files)
	clock := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }

	before, _ := s.Product("CB-1001")
	o, err := s.Place("ken@example.com", []Item{{SKU: "CB-1001", Qty: 1}, {SKU: "CB-1001", Qty: 2}, {SKU: "CM-2001", Qty: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Items) != 2 || o.Items[0].Qty != 3 || o.TotalCents != 3*1450+395 || o.Status != Pending {
		t.Errorf("order = %+v", o)
	}
	if after, _ := s.Product("CB-1001"); after.Stock != before.Stock-3 {
		t.Errorf("stock went from %d to %d", before.Stock, after.Stock)
	}

	for name, tc := range map[string]struct {
		email string
		items []Item
		want  error
	}{
		"no items":      {"ken@example.com", nil, ErrInvalid},
		"bad email":     {"not an address", []Item{{SKU: "CB-1001", Qty: 1}}, ErrInvalid},
		"unknown sku":   {"ken@example.com", []Item{{SKU: "NOPE", Qty: 1}}, ErrNotFound},
		"zero quantity": {"ken@example.com", []Item{{SKU: "CB-1001", Qty: 0}}, ErrInvalid},
		"out of stock":  {"ken@example.com", []Item{{SKU: "IP-3003", Qty: 1}}, ErrConflict},
	} {
		if _, err := s.Place(tc.email, tc.items); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
	}

	// Twenty-nine minutes on, the new order is still pending. The two
	// seeded pending orders are days old and expire.
	clock = clock.Add(29 * time.Minute)
	if n := s.Expire(30 * time.Minute); n != 2 {
		t.Errorf("expired %d orders, want the 2 seeded ones", n)
	}
	if got, _ := s.Order(o.ID); got.Status != Pending {
		t.Errorf("a 29 minute old order is %s", got.Status)
	}
	// Two minutes later it expires, and its items return to stock.
	clock = clock.Add(2 * time.Minute)
	if n := s.Expire(30 * time.Minute); n != 1 {
		t.Errorf("expired %d orders, want 1", n)
	}
	if after, _ := s.Product("CB-1001"); after.Stock != before.Stock {
		t.Errorf("stock is %d after expiry, want %d back", after.Stock, before.Stock)
	}
	if _, err := s.Pay(o.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("paying an expired order: %v", err)
	}

	paid, _ := s.Place("rob@example.com", []Item{{SKU: "DK-5001", Qty: 1}})
	for range 2 { // paying twice gives the same answer
		if got, err := s.Pay(paid.ID); err != nil || got.Status != Paid {
			t.Errorf("pay: %+v, %v", got, err)
		}
	}
}

func TestPlatformContract(t *testing.T) {
	_, srv := testApp(t, config{})
	resp, err := http.Get(srv.URL + "/healthz")
	if got := body(t, resp, err); resp.StatusCode != 200 || got != "ok\n" {
		t.Errorf("/healthz = %d %q", resp.StatusCode, got)
	}
	if resp.Header.Get("X-Cool-Release") != "rtest001" {
		t.Error("responses should name the release that served them")
	}
	http.Get(srv.URL + "/api/products")
	http.Get(srv.URL + "/api/products/NOPE")
	resp, err = http.Get(srv.URL + "/metrics")
	m := body(t, resp, err)
	for _, want := range []string{
		`http_requests_total{code="200",route="GET /api/products"} 1`,
		`http_requests_total{code="404",route="GET /api/products/{sku}"} 1`,
		`http_request_seconds_bucket{`,
	} {
		if !strings.Contains(m, want) {
			t.Errorf("/metrics lacks %s", want)
		}
	}
}

func TestAPI(t *testing.T) {
	_, srv := testApp(t, config{})
	post := func(path, payload string) (*http.Response, string) {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(payload))
		return resp, body(t, resp, err)
	}

	resp, got := post("/api/orders", `{"email":"ken@example.com","items":[{"sku":"IP-3002","qty":2}]}`)
	var o Order
	json.Unmarshal([]byte(got), &o)
	if resp.StatusCode != 201 || o.TotalCents != 5600 || resp.Header.Get("Location") != "/api/orders/"+o.ID {
		t.Fatalf("create: %d %s", resp.StatusCode, got)
	}
	if resp, got := post("/api/orders/"+o.ID+"/pay", ""); resp.StatusCode != 200 || !strings.Contains(got, `"status":"paid"`) {
		t.Errorf("pay: %d %s", resp.StatusCode, got)
	}
	for payload, want := range map[string]int{
		`{"email":"ken@example.com","items":[]}`:                          400,
		`{"email":"ken@example.com","items":[{"sku":"NOPE","qty":1}]}`:    404,
		`{"email":"ken@example.com","items":[{"sku":"IP-3003","qty":1}]}`: 409,
		`{"email":"ken@example.com","items":[], "discount": 100}`:         400,
		`not json`: 400,
	} {
		if resp, got := post("/api/orders", payload); resp.StatusCode != want || !strings.Contains(got, `"error"`) {
			t.Errorf("POST %s = %d %s, want %d and an error", payload, resp.StatusCode, got, want)
		}
	}
	resp, err := http.Get(srv.URL + "/api/orders/ord_missing")
	if body(t, resp, err); resp.StatusCode != 404 {
		t.Errorf("missing order: %d", resp.StatusCode)
	}
}

func TestCron(t *testing.T) {
	call := func(srv *httptest.Server, auth string) (int, string) {
		req, _ := http.NewRequest("POST", srv.URL+"/internal/cron/expire-orders", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		return resp.StatusCode, body(t, resp, err)
	}
	a, srv := testApp(t, config{CronSecret: "s3cret"})
	a.shop.now = func() time.Time { return time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC) }
	for _, auth := range []string{"", "Bearer wrong", "s3cret", "Basic s3cret"} {
		if code, _ := call(srv, auth); code != 401 {
			t.Errorf("cron with %q: %d, want 401", auth, code)
		}
	}
	if code, got := call(srv, "Bearer s3cret"); code != 200 || got != "{\"expired\":2}\n" {
		t.Errorf("cron: %d %s", code, got)
	}
	// With no secret configured the endpoint is closed to everyone.
	_, open := testApp(t, config{})
	if code, _ := call(open, "Bearer "); code != 401 {
		t.Errorf("cron without a configured secret: %d, want 401", code)
	}
}

func TestShopInTheBrowser(t *testing.T) {
	_, srv := testApp(t, config{Flags: map[string]any{"express-checkout": true}})
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}

	resp, err := c.Get(srv.URL + "/")
	page := body(t, resp, err)
	for _, want := range []string{"Cool Beans Coffee", "$14.50", "Out of stock", "Your cart is empty", "rtest001", `hx-post="/cart"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Error("page has no content security policy")
	}

	resp, err = c.Get(srv.URL + "/products?q=" + url.QueryEscape("<script>"))
	if got := body(t, resp, err); strings.Contains(got, "<script>") || !strings.Contains(got, "No product matches") {
		t.Errorf("search fragment does not escape the query: %s", got)
	}

	add := url.Values{"sku": {"CB-1001"}, "qty": {"1"}}
	c.PostForm(srv.URL+"/cart", add)
	resp, err = c.PostForm(srv.URL+"/cart", add)
	if got := body(t, resp, err); !strings.Contains(got, "$29.00") || !strings.Contains(got, "Express checkout") {
		t.Errorf("cart after two adds: %s", got)
	}
	resp, err = c.PostForm(srv.URL+"/cart", url.Values{"sku": {"IP-3003"}})
	if got := body(t, resp, err); !strings.Contains(got, "only 0 in stock") || !strings.Contains(got, "$29.00") {
		t.Errorf("adding an out-of-stock product should report it and keep the cart: %s", got)
	}

	resp, err = c.PostForm(srv.URL+"/checkout", url.Values{"email": {"nobody"}})
	if got := body(t, resp, err); !strings.Contains(got, "is not an address") {
		t.Errorf("checkout with a bad email: %s", got)
	}
	resp, err = c.PostForm(srv.URL+"/checkout", url.Values{"email": {"ken@example.com"}})
	got := body(t, resp, err)
	if !strings.Contains(got, "placed") || !strings.Contains(got, "pending") || resp.Header.Get("HX-Trigger") != "order-changed" {
		t.Errorf("checkout: %s", got)
	}
	resp, err = c.Get(srv.URL + "/cart")
	if got := body(t, resp, err); !strings.Contains(got, "Your cart is empty") {
		t.Errorf("cart after checkout: %s", got)
	}
	resp, err = c.Get(srv.URL + "/orders")
	if got := body(t, resp, err); !strings.Contains(got, "ken") && !strings.Contains(got, "$29.00") {
		t.Errorf("orders after checkout: %s", got)
	}
}

func TestMoney(t *testing.T) {
	usd := &app{cfg: config{Currency: "USD"}}
	eur := &app{cfg: config{Currency: "EUR"}}
	if got := usd.money(1450); got != "$14.50" {
		t.Errorf("usd = %q", got)
	}
	if got := eur.money(5); got != "0.05 EUR" {
		t.Errorf("eur = %q", got)
	}
}
