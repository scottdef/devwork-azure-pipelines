package main

// The HTMX front end. The server renders HTML; the browser swaps fragments.
// There is no client-side state beyond one cookie that holds the cart.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
)

// view is everything the templates can show.
type view struct {
	Products []Product
	Query    string
	Cart     []Item
	Total    int
	Orders   []Order
	Order    *Order // the order just placed or paid, for the confirmation
	Error    string
	Release  string
	Env      string
	Express  bool // the express-checkout feature flag
}

// money formats whole cents: $12.50 for dollars, "12.50 EUR" otherwise.
func (a *app) money(cents int) string {
	s := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if a.cfg.Currency == "USD" {
		return "$" + s
	}
	return s + " " + a.cfg.Currency
}

const cartCookie = "cart"

// cart reads the cart cookie: a base64 JSON object of SKU to quantity. A
// cookie that does not parse is an empty cart.
func (a *app) cart(r *http.Request) map[string]int {
	cart := map[string]int{}
	c, err := r.Cookie(cartCookie)
	if err != nil {
		return cart
	}
	if b, err := base64.RawURLEncoding.DecodeString(c.Value); err == nil {
		json.Unmarshal(b, &cart)
	}
	return cart
}

func (a *app) setCart(w http.ResponseWriter, cart map[string]int) {
	b, _ := json.Marshal(cart)
	c := &http.Cookie{Name: cartCookie, Value: base64.RawURLEncoding.EncodeToString(b), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.Env != "local", MaxAge: 7 * 24 * 3600}
	if len(cart) == 0 {
		c.Value, c.MaxAge = "", -1
	}
	http.SetCookie(w, c)
}

// items turns a cart into order lines, in a stable order.
func items(cart map[string]int) []Item {
	var out []Item
	for sku, qty := range cart {
		out = append(out, Item{SKU: sku, Qty: qty})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out
}

// newView fills in what every page and fragment may need.
func (a *app) newView(r *http.Request, cart map[string]int) view {
	v := view{Release: a.cfg.Release, Env: a.cfg.Env}
	v.Express, _ = a.cfg.Flags["express-checkout"].(bool)
	if len(cart) > 0 {
		priced, total, err := a.shop.Price(items(cart))
		if err != nil {
			v.Error = err.Error()
		}
		v.Cart, v.Total = priced, total
	}
	return v
}

func (a *app) render(w http.ResponseWriter, name string, v view) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.tmpl.ExecuteTemplate(w, name, v); err != nil {
		a.log.Error("render", "template", name, "err", err)
	}
}

func (a *app) page(w http.ResponseWriter, r *http.Request) {
	v := a.newView(r, a.cart(r))
	v.Products = a.shop.Products("")
	v.Orders = a.shop.Orders(5)
	a.render(w, "page", v)
}

func (a *app) productsFragment(w http.ResponseWriter, r *http.Request) {
	v := a.newView(r, nil)
	v.Query = r.URL.Query().Get("q")
	v.Products = a.shop.Products(v.Query)
	a.render(w, "products", v)
}

func (a *app) cartFragment(w http.ResponseWriter, r *http.Request) {
	a.render(w, "cart", a.newView(r, a.cart(r)))
}

func (a *app) cartAdd(w http.ResponseWriter, r *http.Request) {
	cart := a.cart(r)
	sku := r.FormValue("sku")
	qty, err := strconv.Atoi(r.FormValue("qty"))
	if err != nil {
		qty = 1
	}
	cart[sku] += qty
	if cart[sku] <= 0 {
		delete(cart, sku)
	}
	// Price the cart before keeping it: an unknown product or an impossible
	// quantity is reported and the old cart stays.
	if len(cart) > 0 {
		if _, _, err := a.shop.Price(items(cart)); err != nil {
			v := a.newView(r, a.cart(r))
			v.Error = err.Error()
			a.render(w, "cart", v)
			return
		}
	}
	a.setCart(w, cart)
	a.render(w, "cart", a.newView(r, cart))
}

func (a *app) cartClear(w http.ResponseWriter, r *http.Request) {
	a.setCart(w, nil)
	a.render(w, "cart", a.newView(r, nil))
}

func (a *app) checkout(w http.ResponseWriter, r *http.Request) {
	cart := a.cart(r)
	o, err := a.shop.Place(r.FormValue("email"), items(cart))
	if err != nil {
		v := a.newView(r, cart)
		v.Error = err.Error()
		a.render(w, "cart", v)
		return
	}
	if r.FormValue("express") == "1" {
		o, _ = a.shop.Pay(o.ID)
	}
	a.metrics.Inc("checkout_orders_total", "source", "web")
	a.setCart(w, nil)
	// Tell the page that orders changed; the order list listens for it.
	w.Header().Set("HX-Trigger", "order-changed")
	v := a.newView(r, nil)
	v.Order = &o
	a.render(w, "cart", v)
}

func (a *app) ordersFragment(w http.ResponseWriter, r *http.Request) {
	v := a.newView(r, nil)
	v.Orders = a.shop.Orders(5)
	a.render(w, "orders", v)
}

func (a *app) payFragment(w http.ResponseWriter, r *http.Request) {
	v := a.newView(r, nil)
	if _, err := a.shop.Pay(r.PathValue("id")); err != nil {
		v.Error = err.Error()
	}
	v.Orders = a.shop.Orders(5)
	a.render(w, "orders", v)
}
