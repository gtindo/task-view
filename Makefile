.PHONY: build install test vet golden clean

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

clean:
	rm -rf bin dist
