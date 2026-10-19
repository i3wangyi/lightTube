GO ?= mise exec -- go
PREFIX ?= $(HOME)/.local
DESTDIR ?=

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

.PHONY: build test vet fmt check install uninstall clean

build:
	@echo "Building with tags: $(TAGS)"
	$(GO) build $(TAGS_FLAG) -trimpath -ldflags='-s -w' -o lighttube ./cmd/lighttube

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

clean:
	rm -f lighttube
