TIME_VERSION := $(shell date '+%Y%m%d')
GIT_VERSION := $(shell git describe --always --long --dirty)
VERSION := $(TIME_VERSION)-$(GIT_VERSION)
BUILDTIME := $(shell date '+%Y-%m-%d_%H:%M:%S')
BINARY_NAME := temperature-exporter
GOARCH ?= arm64
GOOS ?= linux
GOARM ?=

.PHONE: version
version:
	@echo $(VERSION)

.PHONY: run
run:
	go run cmd/temp-export/main.go

.PHONY: build
build:
	GOARCH=$(GOARCH) GOOS=$(GOOS) go build -o $(BINARY_NAME) -ldflags "-X main.VERSION=$(VERSION) -X main.BUILDTIME=$(BUILDTIME)" cmd/temp-export/main.go
	chmod +x $(BINARY_NAME)
	upx --ultra-brute $(BINARY_NAME)
	mv $(BINARY_NAME) $(BINARY_NAME)-$(GOARCH)$(GOARM)-$(GOOS)
	tar -czvf bin/$(BINARY_NAME)-$(GOARCH)$(GOARM)-$(GOOS)-$(VERSION).tar.gz $(BINARY_NAME)-$(GOARCH)$(GOARM)-$(GOOS)
	mv $(BINARY_NAME)-$(GOARCH)$(GOARM)-$(GOOS) bin/

.PHONY: all
all:
	make amd64-linux
	make arm64-linux
	make arm7-linux
	make arm5-linux

.PHONY: amd64-linux
amd64-linux:
	make build GOARCH=amd64 GOOS=linux

.PHONY: arm64-linux
arm64-linux:
	make build GOARCH=arm64 GOOS=linux

.PHONY: arm7-linux
arm7-linux:
	make build GOARCH=arm GOOS=linux GOARM=7

.PHONY: arm5-linux
arm5-linux:
	make build GOARCH=arm GOOS=linux GOARM=5


.PHONY: clean
clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-*.tar.gz bin/$(BINARY_NAME)-*
