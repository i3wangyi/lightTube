GO ?= mise exec -- go
PREFIX ?= $(HOME)/.local
DESTDIR ?=
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
ARCH ?= $(shell uname -m)

# Auto-detect build tags if not specified on the command line.
# novulkan: added when /usr/include/vulkan/vulkan.h is missing
# nox11: added when x11-xcb, xkbcommon-x11, or xcursor development packages are missing
ifeq ($(origin TAGS), undefined)
  DETECTED_TAGS :=
  ifeq ($(shell test -f /usr/include/vulkan/vulkan.h || echo missing),missing)
    DETECTED_TAGS := novulkan
  endif
  ifeq ($(shell pkg-config --exists x11-xcb xkbcommon-x11 xcursor 2>/dev/null || echo missing),missing)
    ifneq ($(DETECTED_TAGS),)
      DETECTED_TAGS := $(DETECTED_TAGS),nox11
    else
      DETECTED_TAGS := nox11
    endif
  endif
  TAGS := $(DETECTED_TAGS)
endif

TAGS_FLAG := $(if $(strip $(TAGS)),-tags $(strip $(TAGS)))
LDFLAGS := -s -w -X main.version=$(VERSION)

DISTNAME := lighttube-$(VERSION)-linux-$(ARCH)
DISTDIR := dist/$(DISTNAME)
TARBALL := dist/$(DISTNAME).tar.gz

.PHONY: build test vet fmt check install uninstall clean dist

build:
	@echo "Building with tags: $(TAGS)"
	$(GO) build $(TAGS_FLAG) -trimpath -ldflags='$(LDFLAGS)' -o lighttube ./cmd/lighttube

test:
	$(GO) test $(TAGS_FLAG) ./...

vet:
	$(GO) vet $(TAGS_FLAG) ./...

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

dist: build
	@rm -rf $(DISTDIR) $(TARBALL)
	@mkdir -p $(DISTDIR)/contrib dist
	cp lighttube $(DISTDIR)/
	cp contrib/lighttube.desktop $(DISTDIR)/contrib/
	cp contrib/lighttube.svg $(DISTDIR)/contrib/
	cp contrib/hyprland.lua $(DISTDIR)/contrib/
	cp README.md $(DISTDIR)/
	cp scripts/install.sh $(DISTDIR)/install.sh
	chmod +x $(DISTDIR)/install.sh
	tar -czf $(TARBALL) -C dist $(DISTNAME)
	@rm -rf $(DISTDIR)
	@echo "Created $(TARBALL)"

clean:
	rm -rf lighttube dist
