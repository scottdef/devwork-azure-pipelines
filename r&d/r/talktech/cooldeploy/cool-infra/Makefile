# cool-infra: Terraform for everything except API Management.
TF ?= terraform
STACKS := $(sort $(wildcard stacks/*))

.PHONY: check fmt lock values

# check needs no Azure sign-in. It validates each stack when the providers
# can be downloaded, and says so when they cannot.
check:
	TF=$(TF) scripts/check.sh

fmt:
	$(TF) fmt -recursive

# lock writes the provider lock file of every stack. CI runs init with
# -lockfile=readonly, so commit the result.
lock:
	@for s in $(STACKS); do \
		echo "== $$s"; \
		$(TF) -chdir=$$s init -backend=false -input=false >/dev/null && \
		$(TF) -chdir=$$s providers lock -platform=linux_amd64 -platform=linux_arm64 -platform=darwin_arm64; \
	done

# values prints what other repositories need from the applied stacks.
values:
	TF=$(TF) scripts/platform-values.sh
