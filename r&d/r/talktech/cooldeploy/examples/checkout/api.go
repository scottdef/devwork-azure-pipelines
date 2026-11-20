package main

// The JSON API. openapi.yaml describes it; APIM publishes it.

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// fail writes an error in the API's one error shape, with the status that
// fits the kind of error.
func fail(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, ErrInvalid):
		code = http.StatusBadRequest
	case errors.Is(err, ErrConflict):
		code = http.StatusConflict
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func sub(fsys fs.FS, dir string) fs.FS {
	s, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err) // the directory is embedded; this cannot fail at run time
	}
	return s
}

func (a *app) apiProducts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"products": a.shop.Products(r.URL.Query().Get("q"))})
}

func (a *app) apiProduct(w http.ResponseWriter, r *http.Request) {
	p, err := a.shop.Product(r.PathValue("sku"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *app) apiOrders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"orders": a.shop.Orders(50)})
}

func (a *app) apiOrder(w http.ResponseWriter, r *http.Request) {
	o, err := a.shop.Order(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (a *app) apiCreateOrder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Items []Item `json:"items"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body: " + err.Error()})
		return
	}
	o, err := a.shop.Place(in.Email, in.Items)
	if err != nil {
		fail(w, err)
		return
	}
	a.metrics.Inc("checkout_orders_total", "source", "api")
	w.Header().Set("Location", "/api/orders/"+o.ID)
	writeJSON(w, http.StatusCreated, o)
}

func (a *app) apiPay(w http.ResponseWriter, r *http.Request) {
	o, err := a.shop.Pay(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// cronExpire is called by the platform on the schedule in cool.json. The
// caller proves itself with CRON_SECRET; without a configured secret the
// endpoint is closed, not open.
func (a *app) cronExpire(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if a.cfg.CronSecret == "" || !ok || subtle.ConstantTimeCompare([]byte(token), []byte(a.cfg.CronSecret)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "a valid cron token is required"})
		return
	}
	n := a.shop.Expire(30 * time.Minute)
	a.metrics.Add("checkout_orders_expired_total", float64(n))
	a.log.Info("cron", "job", "expire-orders", "expired", n)
	writeJSON(w, http.StatusOK, map[string]int{"expired": n})
}
