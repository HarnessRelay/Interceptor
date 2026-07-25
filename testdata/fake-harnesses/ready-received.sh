#!/bin/sh
printf 'READY\n'
IFS= read -r line
printf 'RECEIVED:%s\n' "$line"
IFS= read -r line
printf 'RECEIVED:%s\n' "$line"
