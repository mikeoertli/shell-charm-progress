.PHONY: build install test check demo

build:
	go build -trimpath -o bin/shell-charm-progress .

install:
	go install -trimpath .

test: build
	go test -race ./...
	python3 tests/test_integration.py

check:
	go vet ./...
	shellcheck -s dash shell/progress.sh examples/*.sh

demo: build
	./bin/shell-charm-progress demo
