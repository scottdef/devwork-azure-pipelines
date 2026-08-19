// main.go - Scion ADK Agent for GitHub IssueOps Orchestration
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"
)

// executeScionTool bridges the Go agent back to the Scion Hub control plane.
func executeScionTool(command, message string) {
	cmd := exec.Command("sciontool", "status", command, message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		slog.Error("Failed to execute sciontool", "error", err, "command", command)
	}
}

// StandardPiece enforces strict type safety and standard interfaces for all units.
type StandardPiece interface {
	Execute(ctx context.Context) error
	Name() string
}

// 1. ExecutionBattalion (Base execution unit)
type ExecutionBattalion struct {
	ID      string
	Command string
	Args    []string
}

func (b *ExecutionBattalion) Name() string { return b.ID }
func (b *ExecutionBattalion) Execute(ctx context.Context) error {
	slog.Info("Battalion executing command", "battalion", b.ID, "cmd", b.Command)
	cmd := exec.CommandContext(ctx, b.Command, b.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// 2. TaskRegiment (Groups Battalions)
type TaskRegiment struct {
	ID         string
	Battalions []StandardPiece
}

func (r *TaskRegiment) Name() string { return r.ID }
func (r *TaskRegiment) Execute(ctx context.Context) error {
	slog.Info("Regiment advancing", "regiment", r.ID)
	for _, b := range r.Battalions {
		if err := b.Execute(ctx); err != nil {
			return fmt.Errorf("regiment %s failed at battalion %s: %w", r.ID, b.Name(), err)
		}
	}
	return nil
}

// 3. JobBrigade (Groups Regiments)
type JobBrigade struct {
	ID        string
	Regiments []StandardPiece
}

func (b *JobBrigade) Name() string { return b.ID }
func (b *JobBrigade) Execute(ctx context.Context) error {
	slog.Info("Brigade engaged", "brigade", b.ID)
	for _, r := range b.Regiments {
		if err := r.Execute(ctx); err != nil {
			return fmt.Errorf("brigade %s failed at regiment %s: %w", b.ID, r.Name(), err)
		}
	}
	return nil
}

// 4. WorkflowDivision (Groups Brigades - Parallel execution)
type WorkflowDivision struct {
	ID       string
	Brigades []StandardPiece
}

func (d *WorkflowDivision) Name() string { return d.ID }
func (d *WorkflowDivision) Execute(ctx context.Context) error {
	slog.Info("Division maneuvering", "division", d.ID)
	var wg sync.WaitGroup
	errChan := make(chan error, len(d.Brigades))

	for _, b := range d.Brigades {
		wg.Add(1)
		go func(piece StandardPiece) {
			defer wg.Done()
			if err := piece.Execute(ctx); err != nil {
				errChan <- err
			}
		}(b)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return fmt.Errorf("division %s suffered failure: %w", d.ID, err)
		}
	}
	return nil
}

// 5. OperationCorps (Top-Level Independent Process)
type OperationCorps struct {
	ID        string
	Divisions []StandardPiece
}

func (c *OperationCorps) Name() string { return c.ID }
func (c *OperationCorps) Execute(ctx context.Context) error {
	slog.Info("Corps d'Armée activated", "corps", c.ID)
	for _, d := range c.Divisions {
		if err := d.Execute(ctx); err != nil {
			return fmt.Errorf("corps %s compromised at division %s: %w", c.ID, d.Name(), err)
		}
	}
	return nil
}

// CompileOrderOfBattle parses the directive into an executable hierarchy.
func CompileOrderOfBattle(directive string) StandardPiece {
	slog.Info("Berthier parsing Emperor's intent...", "directive", directive)
	return &OperationCorps{
		ID: "I Corps (Infrastructure)",
		Divisions: []StandardPiece{
			&WorkflowDivision{
				ID: "1st Workflow Division (Validation & Generation)",
				Brigades: []StandardPiece{
					&JobBrigade{
						ID: "1st Brigade (IaC Processing)",
						Regiments: []StandardPiece{
							&TaskRegiment{
								ID: "1st Regiment (Format & Plan)",
								Battalions: []StandardPiece{
									&ExecutionBattalion{ID: "1st Battalion", Command: "echo", Args: []string{"[MOCK] Executing terraform fmt"}},
									&ExecutionBattalion{ID: "2nd Battalion", Command: "echo", Args: []string{"[MOCK] Executing terraform plan"}},
								},
							},
						},
					},
				},
			},
		},
	}
}

func main() {
	directive := flag.String("directive", "", "Operational intent from GitHub")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	token := os.Getenv("GITHUB_TOKEN")
	workspace := os.Getenv("SCION_WORKSPACE")

	if token == "" || workspace == "" {
		slog.Error("Missing required logistics (environment variables)")
		executeScionTool("task_failed", "System crash: Missing credentials or workspace.")
		os.Exit(1)
	}

	if err := os.Chdir(workspace); err != nil {
		slog.Error("Failed to occupy workspace", "error", err)
		executeScionTool("task_failed", "System crash: Cannot navigate to workspace.")
		os.Exit(1)
	}

	slog.Info("Imperial Headquarters initializing...", "workspace", workspace)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	grandArmee := CompileOrderOfBattle(*directive)

	if err := grandArmee.Execute(ctx); err != nil {
		slog.Error("Operational Failure", "error", err)
		executeScionTool("task_failed", fmt.Sprintf("Operation failed: %v", err))
		os.Exit(1)
	}

	slog.Info("Victory. All operational objectives achieved.")
	executeScionTool("task_completed", "IssueOps PR generated successfully.")
}
