#!/usr/bin/env bash
# refresh source files from main repo (one level up).
# this intends to be same as main except removing `tail` 3rd party package.
set -e

rsync -i ../main.go .
rsync -i ../postfix_exporter.go .
