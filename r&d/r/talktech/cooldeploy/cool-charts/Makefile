# cool-charts: render and check the charts.
COOLCTL ?= coolctl

.PHONY: check render

check:
	COOLCTL=$(COOLCTL) test/render.sh

# render keeps the YAML, for reading.
render:
	COOLCTL=$(COOLCTL) OUT=rendered test/render.sh
