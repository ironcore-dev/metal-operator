#!/usr/bin/env bash

set -e

BASEDIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
export TERM="xterm-256color"

bold="$(tput bold)"
red="$(tput setaf 1)"
green="$(tput setaf 2)"
normal="$(tput sgr0)"

# Determine kustomize command once before loop
if command -v kustomize &> /dev/null; then
  kustomize_cmd="kustomize build"
elif command -v kubectl &> /dev/null; then
  kustomize_cmd="kubectl kustomize"
else
  echo "${red}Error: Neither 'kustomize' nor 'kubectl' found in PATH${normal}"
  exit 1
fi

for kustomization in "$BASEDIR"/../config/**/kustomization.yaml; do
  path="$(dirname "$kustomization")"
  # Get relative path (works on both macOS and Linux)
  dir="${path#"$BASEDIR"/../}"
  echo "${bold}Validating $dir${normal}"
  if ! kustomize_output="$($kustomize_cmd "$path" 2>&1)"; then
    echo "${red}Kustomize build $dir failed:"
    echo "$kustomize_output"
    exit 1
  fi
  echo "${green}Successfully validated $dir${normal}"
done
