.PHONY: build install test vet lint check golden docs demo release clean bench-prepare bench bench-report bench-publish

VERSION ?= 0.1.0
ESLINT := npx --yes eslint@10.12.0
LDFLAGS := -s -w -X main.version=$(VERSION)
DEMO_COMMIT := 12199940391020372c51b1f669dfa83b4f4c8bee

build:
	go build -o bin/task-view ./cmd/task-view

install:
	go install ./cmd/task-view

test:
	go test ./...
	cd bench && go test ./...

vet:
	go vet ./...
	cd bench && go vet ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

# Code rules from AGENTS.md: golangci-lint for Go, core ESLint rules for app.js.
lint:
	golangci-lint run ./...
	$(ESLINT) -c eslint.config.mjs internal/render/assets/app.js

check: vet lint test

golden:
	go test ./cmd/task-view -run 'TestValidReviewGolden|TestGatherGolden' -update

# docs/TASK_REVIEW.md is generated from the embedded copy in
# internal/instructions so the two never drift; edit that one instead.
docs:
	go run ./cmd/task-view instructions > docs/TASK_REVIEW.md

# The live demo in site/: the reviews in demo/ describe DEMO_COMMIT of this
# repo. They are opened in a throwaway clone checked out at that commit, so
# the page matches the state the agent described and carries no head_ahead
# warning for the commits made since. change-broken.json is wrong on purpose:
# its page renders with exit 1 and the red error banner.
demo:
	go build -ldflags "$(LDFLAGS)" -o bin/task-view ./cmd/task-view
	rm -rf site && mkdir -p site
	tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
	git clone --quiet --no-checkout . "$$tmp/repo" && \
	git -C "$$tmp/repo" checkout --quiet --detach $(DEMO_COMMIT) && \
	mkdir "$$tmp/repo/.review" && cp demo/*.json "$$tmp/repo/.review/" && \
	bin/task-view open "$$tmp/repo/.review/change-show-code-after.json" --no-browser --out "$(CURDIR)/site/index.html" && \
	{ bin/task-view open "$$tmp/repo/.review/change-broken.json" --no-browser --out "$(CURDIR)/site/broken.html" || test $$? -eq 1; }

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

# The benchmark in bench/ (its own Go module; see bench/README.md). It calls
# models and costs money, so it is never part of check. BENCH_ARGS passes run
# flags, e.g. make bench BENCH_ARGS="-cases httpx-* -models haiku -repeats 1".
bench-prepare:
	cd bench && go run ./cmd/bench prepare

bench:
	cd bench && go run ./cmd/bench run $(BENCH_ARGS)

bench-report:
	cd bench && go run ./cmd/bench report $(or $(BENCH_OUT),.cache/results/latest)

# Replaces bench/results/ and BENCH.md with the run in BENCH_OUT: the repo
# keeps only the latest published run. BENCH_DASHBOARD is the shared page
# that draws the results as charts; update its data when a new run lands.
BENCH_DASHBOARD ?= https://claude.ai/artifact/J5NQkrcbKVjaX6gSLDGs9g

bench-publish:
	cd bench && go run ./cmd/bench publish -dashboard "$(BENCH_DASHBOARD)" $(or $(BENCH_OUT),.cache/results/latest)

clean:
	rm -rf bin dist release site
