# p340-strata: one interface for the prototype and production planes.
# Go 1.24, standard library only.

SHELL := /bin/bash
BIN   := bin
UNITS := p340-gate p340-brain-llama p340-brain-strata p340-embed
U     := systemctl --user

.PHONY: help build test sim preflight plan install-units prod-llama prod-strata proto status checksums clean

help: ## list targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-14s %s\n", $$1, $$2}'

build: ## compile gate, council, agent and p340sim into ./bin
	@mkdir -p $(BIN)
	go build -trimpath -ldflags='-s -w' -o $(BIN)/ ./cmd/...

test: ## vet and run the unit tests (mock servers, no GPU needed)
	gofmt -l . | (! grep .)
	go vet ./...
	go test ./...

sim: build ## rerun the five-pattern model and save the table
	$(BIN)/p340sim | tee sim/results.md
	$(BIN)/p340sim -format json > sim/results.json

preflight: ## read-only check of this machine
	@bash scripts/preflight.sh

plan: ## ask Strata whether it accepts this machine (changes nothing)
	@bash scripts/strata-plan.sh

install-units: build ## link the systemd user units
	@test -f $$HOME/.config/p340/env || { echo "copy etc/env.example to ~/.config/p340/env first"; exit 1; }
	@mkdir -p $$HOME/.config/systemd/user
	@for u in $(UNITS); do ln -sf $(CURDIR)/systemd/$$u.service $$HOME/.config/systemd/user/; done
	$(U) daemon-reload

prod-llama: ## production plane on llama-server (works on the stock P340)
	$(U) start p340-brain-llama p340-embed p340-gate

prod-strata: ## production plane on Strata (needs a card Strata accepts)
	$(U) start p340-brain-strata p340-embed p340-gate

proto: ## free RAM and VRAM for Unsloth Studio
	-$(U) stop p340-brain-llama p340-brain-strata
	@echo "brain lanes stopped; start Studio with: unsloth studio"

status: ## what is running, and what the GPU holds
	-@$(U) --no-pager --lines=0 status $(UNITS) 2>/dev/null | grep -E '●|Active'
	-@nvidia-smi --query-gpu=memory.used,memory.total --format=csv,noheader
	-@curl -s -m 2 http://127.0.0.1:4000/v1/models -H "Authorization: Bearer $$GATE_KEY"; echo

checksums: ## write SHA256SUMS for every tracked file
	@find . -type f ! -path './bin/*' ! -name SHA256SUMS | sort | xargs sha256sum > SHA256SUMS
	@echo "wrote SHA256SUMS ($$(wc -l < SHA256SUMS) files)"

clean: ## remove ./bin
	rm -rf $(BIN)
