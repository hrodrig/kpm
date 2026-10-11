# BSD Make stub: forward to gmake (the canonical GNUmakefile).
# Usage: make [target]   (requires gmake installed; pkg install gmake)

.DEFAULT_GOAL := help

help:
	@echo "This project uses GNU Make. Run: gmake [target]"
	@echo ""
	@echo "Install gmake:  pkg install gmake (FreeBSD) / doas pkg_add gmake (OpenBSD)"

%:
	@gmake $(MAKECMDGOALS)
