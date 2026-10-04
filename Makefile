UNIT_DIR := $(HOME)/.config/systemd/user
PLUGIN_DIR := $(HOME)/.config/omarchy/plugins/champip.nexus
PAUSE_PLUGIN_DIR := $(HOME)/.config/omarchy/plugins/champip.nexus-pausa

.PHONY: build test install uninstall-service
build:
	go build -o nexus ./cmd/nexus

# The architecture test inspects other packages via `go list`, so its result must never come from the test cache.
test:
	go vet ./... && go test ./... && go test -count=1 ./internal/archtest

# Installs the binary and links the bar plugin (edits here hot-reload in the shell).
install:
	go build -o $(HOME)/.local/bin/nexus ./cmd/nexus
	ln -sfn $(CURDIR)/plugin/champip.nexus $(PLUGIN_DIR)
	ln -sfn $(CURDIR)/plugin/champip.nexus-pausa $(PAUSE_PLUGIN_DIR)
	mkdir -p $(UNIT_DIR)
	cp contrib/systemd/nexus.service $(UNIT_DIR)/nexus.service
	systemctl --user daemon-reload
	systemctl --user enable nexus.service
	systemctl --user restart nexus.service
	-omarchy-shell shell rescanPlugins
	-omarchy-shell shell enablePlugin champip.nexus-pausa '{}'
	@echo "Now run: omarchy plugin enable champip.nexus"

# Detiene y desactiva el servicio de avisos y borra su unidad.
uninstall-service:
	-systemctl --user disable --now nexus.service
	rm -f $(UNIT_DIR)/nexus.service
	systemctl --user daemon-reload
