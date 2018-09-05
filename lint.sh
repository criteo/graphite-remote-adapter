#!/usr/bin/env bash

set -e

if [[ -z "$1" ]]; then
    GO_EXEC="go"
else
    GO_EXEC=$1
fi

LINTABLE=$(${GO_EXEC} list ./... | grep -v graphite-remote-adapter/ui)

echo "Checking golint..."
lint_result=$(echo $LINTABLE | xargs -n 1 golint)
if [ -n "${lint_result}" ]; then
  echo -e "golint checking failed:\n${lint_result}"
  exit 2
fi
