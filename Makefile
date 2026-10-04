PLUGIN_DIR := $(HOME)/.config/omarchy/plugins/champip.nexus

.PHONY: build test install
build:
	go build -o nexus ./cmd/nexus

test:
	go vet ./... && go test ./...

# Installs the binary and links the bar plugin (edits here hot-reload in the shell).
install:
	go build -o $(HOME)/.local/bin/nexus ./cmd/nexus
	ln -sfn $(CURDIR)/plugin/champip.nexus $(PLUGIN_DIR)
	-omarchy-shell shell rescanPlugins
	@echo "Now run: omarchy plugin enable champip.nexus"
