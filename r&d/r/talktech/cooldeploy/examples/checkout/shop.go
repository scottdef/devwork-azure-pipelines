package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/mail"
	"sort"
	"strings"
	"sync"
	"time"
)

// Product is one thing the shop sells. Prices are whole cents: money is
// never a float.
type Product struct {
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	PriceCents  int    `json:"price_cents"`
	Stock       int    `json:"stock"`
}

// Item is one line of a cart or an order.
type Item struct {
	SKU       string `json:"sku"`
	Name      string `json:"name,omitempty"`
	Qty       int    `json:"qty"`
	UnitCents int    `json:"unit_cents,omitempty"`
}

// Order statuses.
const (
	Pending = "pending"
	Paid    = "paid"
	Expired = "expired"
)

// Order is a placed order.
type Order struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	Items      []Item    `json:"items"`
	TotalCents int       `json:"total_cents"`
	Status     string    `json:"status"`
	Created    time.Time `json:"created"`
}

// Errors the handlers translate into HTTP statuses.
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
	ErrConflict = errors.New("conflict")
)

// Limits on one order.
const (
	maxLines = 20
	maxQty   = 99
)

// shop is the catalogue and the orders.
//
// It keeps everything in memory, seeded from the embedded sample data, which
// is right for a demo and wrong for a shop: each replica has its own orders
// and a restart forgets them. A real app keeps this type's methods and puts
// a database behind them; nothing outside this file would change.
type shop struct {
	mu       sync.RWMutex
	products map[string]Product
	orders   map[string]*Order
	now      func() time.Time
}

// newShop loads data/products.json and data/orders.json from fsys.
func newShop(fsys fs.FS) (*shop, error) {
	s := &shop{products: map[string]Product{}, orders: map[string]*Order{}, now: time.Now}
	var products []Product
	if err := readJSON(fsys, "data/products.json", &products); err != nil {
		return nil, err
	}
	for _, p := range products {
		if p.SKU == "" || p.PriceCents <= 0 {
			return nil, fmt.Errorf("data/products.json: product %q needs a sku and a price", p.Name)
		}
		s.products[p.SKU] = p
	}
	var orders []*Order
	if err := readJSON(fsys, "data/orders.json", &orders); err != nil {
		return nil, err
	}
	for _, o := range orders {
		s.orders[o.ID] = o
	}
	return s, nil
}

func readJSON(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Products returns the products whose name, category or SKU contains q,
// sorted by name. An empty q matches everything.
func (s *shop) Products(q string) []Product {
	q = strings.ToLower(strings.TrimSpace(q))
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Product
	for _, p := range s.products {
		if q == "" || strings.Contains(strings.ToLower(p.Name+" "+p.Category+" "+p.SKU), q) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Product returns one product.
func (s *shop) Product(sku string) (Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.products[sku]
	if !ok {
		return Product{}, fmt.Errorf("product %q: %w", sku, ErrNotFound)
	}
	return p, nil
}

// Price fills in names and unit prices for the items and returns the total.
// Items with the same SKU are merged. It refuses unknown products, bad
// quantities and orders with too many lines.
func (s *shop) Price(items []Item) ([]Item, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.price(items)
}

func (s *shop) price(items []Item) ([]Item, int, error) {
	qty := map[string]int{}
	var order []string
	for _, it := range items {
		if _, seen := qty[it.SKU]; !seen {
			order = append(order, it.SKU)
		}
		qty[it.SKU] += it.Qty
	}
	if len(order) == 0 {
		return nil, 0, fmt.Errorf("an order needs at least one item: %w", ErrInvalid)
	}
	if len(order) > maxLines {
		return nil, 0, fmt.Errorf("an order holds at most %d different products: %w", maxLines, ErrInvalid)
	}
	var out []Item
	total := 0
	for _, sku := range order {
		p, ok := s.products[sku]
		switch n := qty[sku]; {
		case !ok:
			return nil, 0, fmt.Errorf("product %q: %w", sku, ErrNotFound)
		case n < 1 || n > maxQty:
			return nil, 0, fmt.Errorf("%s: quantity must be 1 to %d: %w", sku, maxQty, ErrInvalid)
		case n > p.Stock:
			return nil, 0, fmt.Errorf("%s: only %d in stock: %w", sku, p.Stock, ErrConflict)
		default:
			out = append(out, Item{SKU: sku, Name: p.Name, Qty: n, UnitCents: p.PriceCents})
			total += n * p.PriceCents
		}
	}
	return out, total, nil
}

// Place creates a pending order and takes its items out of stock.
func (s *shop) Place(email string, items []Item) (Order, error) {
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return Order{}, fmt.Errorf("email %q is not an address: %w", email, ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	priced, total, err := s.price(items)
	if err != nil {
		return Order{}, err
	}
	for _, it := range priced {
		p := s.products[it.SKU]
		p.Stock -= it.Qty
		s.products[it.SKU] = p
	}
	o := &Order{ID: newID(), Email: email, Items: priced, TotalCents: total, Status: Pending, Created: s.now().UTC()}
	s.orders[o.ID] = o
	return *o, nil
}

// Order returns one order.
func (s *shop) Order(id string) (Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o, ok := s.orders[id]
	if !ok {
		return Order{}, fmt.Errorf("order %q: %w", id, ErrNotFound)
	}
	return *o, nil
}

// Orders returns the newest n orders, newest first.
func (s *shop) Orders(n int) []Order {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Order, 0, len(s.orders))
	for _, o := range s.orders {
		out = append(out, *o)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Created.Equal(out[j].Created) {
			return out[i].Created.After(out[j].Created)
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Pay marks a pending order paid. Paying twice is not an error: a client
// that retries must get the same answer.
func (s *shop) Pay(id string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	switch {
	case !ok:
		return Order{}, fmt.Errorf("order %q: %w", id, ErrNotFound)
	case o.Status == Expired:
		return Order{}, fmt.Errorf("order %s has expired: %w", id, ErrConflict)
	}
	o.Status = Paid
	return *o, nil
}

// Expire marks pending orders older than age as expired, returns their
// items to stock and reports how many it expired.
func (s *shop) Expire(age time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := s.now().Add(-age)
	n := 0
	for _, o := range s.orders {
		if o.Status != Pending || o.Created.After(cutoff) {
			continue
		}
		o.Status = Expired
		for _, it := range o.Items {
			if p, ok := s.products[it.SKU]; ok {
				p.Stock += it.Qty
				s.products[it.SKU] = p
			}
		}
		n++
	}
	return n
}

// newID returns an order ID such as ord_5f2c9a1b3d4e.
func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return "ord_" + hex.EncodeToString(b)
}
