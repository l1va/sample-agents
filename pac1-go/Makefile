# AICODE-NOTE: Mirrors pac1-py/Makefile so a fresh checkout stays trivial to run.
# The gen/ directory is regenerated from ../proto via buf.gen.yaml. It is
# committed, so `make tidy && make run` works without running `make gen` first.

.PHONY: gen tidy build run task

gen:
	buf generate

tidy:
	go mod tidy

build:
	go build ./...

run:
	go run .

task:
	@if [ -z "$(TASKS)" ]; then echo "usage: make task TASKS='t01 t03'"; exit 1; fi
	go run . $(TASKS)
