.PHONY: build test clean

BINARY := goralph
TARGET_DIR := target

build:
	mkdir -p $(TARGET_DIR)
	go build -o $(TARGET_DIR)/$(BINARY) ./cmd/goralph

test:
	go test ./...

clean:
	rm -rf $(TARGET_DIR)
