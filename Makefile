.PHONY: run-kanban build
run-kanban:
	npx vibe-kanban

build:
	$(MAKE) -C backend build