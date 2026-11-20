# cool-actions: lint the workflows and actions, bundle coolctl.
COOLCTL_SRC ?= ../coolctl
VERSION     ?= $(shell git -C $(COOLCTL_SRC) describe --tags --always 2>/dev/null || echo dev)

.PHONY: check bundle

# actionlint reads the workflows; the composite actions are checked for
# valid YAML and their embedded scripts by actionlint's shellcheck pass where
# it can see them.
check:
	actionlint -color .github/workflows/*.yml
	@for f in */action.yml; do \
		python3 -c "import sys, yaml; d = yaml.safe_load(open(sys.argv[1])); assert d['runs']['using'] == 'composite', sys.argv[1]" "$$f" \
			|| { echo "bad action: $$f"; exit 1; }; \
	done
	@! grep -rnE 'uses: +[^#]+@(v[0-9]+|main|master) *$$' .github/workflows */action.yml | grep -v 'CoolEngOrg/' \
		|| { echo "pin third-party actions to a commit SHA"; exit 1; }
	@echo "cool-actions: ok"

# bundle builds coolctl for the runners' architectures into bin/, for
# runners whose image does not carry it. Commit the result before tagging.
bundle:
	cd $(COOLCTL_SRC) && for arch in amd64 arm64; do \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 go build -trimpath \
			-ldflags="-s -w -X main.version=$(VERSION)" \
			-o $(CURDIR)/bin/coolctl-linux-$$arch ./cmd/coolctl; \
	done
	cd bin && sha256sum coolctl-linux-* > SHA256SUMS
