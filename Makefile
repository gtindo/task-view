.PHONY: build install test vet lint check golden docs release clean

VERSION ?= 0.1.0
ESLINT := npx --yes eslint@10.12.0
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -o bin/change-view ./cmd/change-view

install:
	go install ./cmd/change-view

test:
	go test ./...

vet:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

# Code rules from AGENTS.md: golangci-lint for Go, core ESLint rules for app.js.
lint:
	golangci-lint run ./...
	$(ESLINT) -c eslint.config.mjs internal/render/assets/app.js

check: vet lint test

golden:
	go test ./cmd/change-view -run TestValidReviewGolden -update

# docs/CHANGE_REVIEW.md is generated from the embedded copy in
# internal/instructions so the two never drift; edit that one instead.
docs:
	go run ./cmd/change-view instructions > docs/CHANGE_REVIEW.md

# Cross-compiled binaries plus the guides from docs/, in release/.
release: docs
	mkdir -p release
	cp docs/CHANGE_REVIEW.md docs/USER_GUIDE.md release/
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/change-view-linux-amd64 ./cmd/change-view
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/change-view-darwin-amd64 ./cmd/change-view
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/change-view-darwin-arm64 ./cmd/change-view
	cd release && shasum -a 256 change-view-* > SHA256SUMS

clean:
	rm -rf bin dist release
