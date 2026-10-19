GO ?= mise exec -- go
TAGS = novulkan,nox11
PREFIX ?= $(HOME)/.local
DESTDIR ?=

.PHONY: build test vet fmt check install uninstall clean

build:
	$(GO) build -tags $(TAGS) -trimpath -ldflags='-s -w' -o lighttube ./cmd/lighttube

test:
	$(GO) test -tags $(TAGS) ./...

vet:
	$(GO) vet -tags $(TAGS) ./...

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

check: fmt vet test

install: build
	install -Dm755 lighttube $(DESTDIR)$(PREFIX)/bin/lighttube
	install -Dm644 contrib/lighttube.desktop $(DESTDIR)$(PREFIX)/share/applications/lighttube.desktop
	install -Dm644 contrib/lighttube.svg $(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps/lighttube.svg
	@if [ -z "$(DESTDIR)" ] && command -v update-desktop-database >/dev/null 2>&1; then \
		update-desktop-database $(PREFIX)/share/applications; \
	fi

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/lighttube
	rm -f $(DESTDIR)$(PREFIX)/share/applications/lighttube.desktop
	rm -f $(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps/lighttube.svg

clean:
	rm -f lighttube
