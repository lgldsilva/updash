#!/usr/bin/env bash
# git-push.sh — push com gates do ai-standards + validate local do projeto.
# Uso: ./git-push.sh [remote] [branch]
export COVERAGE_MIN="${COVERAGE_MIN:-60}"
export COVERAGE_EXCLUDE="${COVERAGE_EXCLUDE:-/(cmd|platform|updater|deploy|cli|upgrade)/}"
exec git push "$@"
