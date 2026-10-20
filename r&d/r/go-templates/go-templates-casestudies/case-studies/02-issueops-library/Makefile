# Case study 02: drives both repositories. Each repo also builds alone.
REPOS := issueops-intake issueops-library

.PHONY: all build test vet lint cross release preflight loadtest site-sample clean

all build test vet lint cross release clean:
	@for r in $(REPOS); do echo "== $$r: $@"; $(MAKE) -C $$r $@ || exit 1; done

preflight:
	@for r in $(REPOS); do echo "== $$r: preflight"; (cd $$r && scripts/preflight.sh) || exit 1; done
	@for f in scripts/*.sh; do bash -n "$$f"; done
	@echo "case study 02: preflight ok"

site-sample:
	$(MAKE) -C issueops-library site-sample

loadtest:
	scripts/loadtest.sh
