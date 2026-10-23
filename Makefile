.PHONY: build install test vet golden release clean

VERSION ?= 0.1.0
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

golden:
	go test ./cmd/change-view -run TestValidReviewGolden -update

# Cross-compiled binaries plus the guides, in release/.
release:
	mkdir -p release
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/change-view-linux-amd64 ./cmd/change-view
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/change-view-darwin-amd64 ./cmd/change-view
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o release/change-view-darwin-arm64 ./cmd/change-view
	go run ./cmd/change-view instructions > release/CHANGE_REVIEW.md
	cd release && shasum -a 256 change-view-* > SHA256SUMS

clean:
	rm -rf bin dist release/change-view-* release/SHA256SUMS
