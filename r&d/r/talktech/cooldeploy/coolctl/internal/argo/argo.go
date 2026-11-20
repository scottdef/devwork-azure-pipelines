// Package argo reads the state of Argo CD Applications.
//
// cool-api runs on the hub cluster beside Argo CD, so it asks the Kubernetes
// API for the Application objects directly, with its service-account token.
// That needs no Argo CD credentials and no client library.
package argo

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Status is the part of an Application's status the platform reports.
type Status struct {
	Sync     string `json:"sync"`   // Synced, OutOfSync or Unknown
	Health   string `json:"health"` // Healthy, Progressing, Degraded, Missing or Unknown
	Phase    string `json:"phase,omitempty"`
	Revision string `json:"revision,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Ready reports whether the Application is synced and healthy.
func (s Status) Ready() bool { return s.Sync == "Synced" && s.Health == "Healthy" }

// Client reads and refreshes Applications.
type Client interface {
	Status(ctx context.Context, application string) (Status, error)
	Refresh(ctx context.Context, application string) error
}

// Name returns the Application name of an app in an environment, or of a
// pull-request preview when pr is positive.
func Name(team, app, env string, pr int) string {
	if pr > 0 {
		return fmt.Sprintf("%s-%s-pr-%d", team, app, pr)
	}
	return team + "-" + app + "-" + env
}

// None is the Client for places with no Argo CD: tests and the local demo.
// It reports every Application as Unknown.
type None struct{}

// Status implements Client.
func (None) Status(context.Context, string) (Status, error) {
	return Status{Sync: "Unknown", Health: "Unknown", Message: "no Argo CD is configured"}, nil
}

// Refresh implements Client.
func (None) Refresh(context.Context, string) error { return nil }

// InCluster talks to the API server of the cluster the process runs in.
type InCluster struct {
	Namespace string // where Applications live; default argocd
	host      string
	token     string // path of the service-account token; re-read on each call
	http      *http.Client
}

const saDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// NewInCluster builds a client from the pod's service account.
func NewInCluster(namespace string) (*InCluster, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("not running in a cluster: KUBERNETES_SERVICE_HOST is unset")
	}
	ca, err := os.ReadFile(saDir + "/ca.crt")
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("no certificates in the service-account CA file")
	}
	if namespace == "" {
		namespace = "argocd"
	}
	return &InCluster{
		Namespace: namespace,
		host:      "https://" + net.JoinHostPort(host, port),
		token:     saDir + "/token",
		http: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
	}, nil
}

func (c *InCluster) do(ctx context.Context, method, application, contentType string, body []byte, out any) error {
	token, err := os.ReadFile(c.token) // the kubelet rotates this file
	if err != nil {
		return err
	}
	url := c.host + "/apis/argoproj.io/v1alpha1/namespaces/" + c.Namespace + "/applications/" + application
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("application %s: not found", application)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("application %s: %s: %s", application, resp.Status, bytes.TrimSpace(b))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// Status implements Client.
func (c *InCluster) Status(ctx context.Context, application string) (Status, error) {
	var app struct {
		Status struct {
			Sync           struct{ Status, Revision string }
			Health         struct{ Status, Message string }
			OperationState struct{ Phase, Message string }
		}
	}
	if err := c.do(ctx, http.MethodGet, application, "", nil, &app); err != nil {
		return Status{}, err
	}
	s := Status{
		Sync:     app.Status.Sync.Status,
		Health:   app.Status.Health.Status,
		Phase:    app.Status.OperationState.Phase,
		Revision: app.Status.Sync.Revision,
		Message:  app.Status.Health.Message,
	}
	if s.Message == "" {
		s.Message = app.Status.OperationState.Message
	}
	return s, nil
}

// Refresh asks Argo CD to compare the Application with Git now instead of at
// its next poll. It sets the annotation the Argo CD CLI sets.
func (c *InCluster) Refresh(ctx context.Context, application string) error {
	patch := []byte(`{"metadata":{"annotations":{"argocd.argoproj.io/refresh":"normal"}}}`)
	return c.do(ctx, http.MethodPatch, application, "application/merge-patch+json", patch, nil)
}
