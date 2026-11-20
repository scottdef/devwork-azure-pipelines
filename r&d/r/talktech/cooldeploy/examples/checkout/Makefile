.PHONY: run test check dockerfile

run:
	COOL_FLAGS_FILE=flags.local.json go run .

test:
	go test ./...

check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet ./...
	go test ./...
	coolctl validate

# Print the Dockerfile the go-htmx preset builds this app with.
dockerfile:
	coolctl render dockerfile
