# GOROOT is unset for every recipe on purpose.
#
# This machine exports GOROOT pointing at one Go installation while a different
# go binary comes first on PATH, and the two disagree on patch version, which
# makes every build fail with "does not match go tool version". Letting the go
# binary find its own GOROOT is correct for any installation, so this costs
# nothing on a machine that is not misconfigured.
GO := env -u GOROOT go

BINARY := flashtui
PKGS := ./...

.PHONY: all build test race cover vet fmt fmt-check tidy clean check run

all: check build

build:
	$(GO) build -trimpath -o $(BINARY) ./cmd/flashtui

run: build
	./$(BINARY)

test:
	$(GO) test $(PKGS)

race:
	$(GO) test -race $(PKGS)

cover:
	$(GO) test -coverprofile=coverage.out -covermode=atomic $(PKGS)
	$(GO) tool cover -func=coverage.out | tail -1

vet:
	$(GO) vet $(PKGS)

fmt:
	$(GO) fmt $(PKGS)

fmt-check:
	@test -z "$$($(GO) fmt $(PKGS))" || { echo "gofmt needed"; exit 1; }

tidy:
	$(GO) mod tidy

clean:
	rm -f $(BINARY) coverage.out

check: fmt-check vet test
