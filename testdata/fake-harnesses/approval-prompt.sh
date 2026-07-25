#!/bin/sh
printf 'approval required\n'
printf 'command: git status --short\n'
printf 'approve? [y/N] '
IFS= read -r answer
case "$answer" in
  y|Y|yes|YES)
    printf 'approved\n'
    exit 0
    ;;
  *)
    printf 'denied\n'
    exit 2
    ;;
esac
