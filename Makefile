.PHONY: build install test vet lint check golden docs release clean

VERSION ?= 0.1.0
ESLINT := npx --yes eslint@10.12.0
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -o bin/task-view ./cmd/task-view

install:
	go install ./cmd/task-view

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
	go test ./cmd/task-view -run TestValidReviewGolden -update

# docs/TASK_REVIEW.md is generated from the embedded copy in
# internal/instructions so the two never drift; edit that one instead.
docs:
	go run ./cmd/task-view instructions > docs/TASK_REVIEW.md

# Cross-compiled binaries plus the guides from docs/, the agent skill and
# the licenses (the page embeds vendored libraries), in release/. The
# tag-triggered workflow in .github/workflows/release.yml uploads this folder
# as the GitHub release.
release: docs
	mkdir -p release
	cp docs/TASK_REVIEW.md docs/USER_GUIDE.md skills/task-review/SKILL.md release/
	cp LICENSE release/
	cp internal/render/assets/vendor/LICENSES.txt release/THIRD_PARTY_LICENSES.txt
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/task-view-linux-amd64 ./cmd/task-view
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/task-view-linux-arm64 ./cmd/task-view
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/task-view-windows-amd64.exe ./cmd/task-view
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/task-view-darwin-amd64 ./cmd/task-view
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/task-view-darwin-arm64 ./cmd/task-view
	cd release && shasum -a 256 task-view-* > SHA256SUMS

clean:
	rm -rf bin dist release
