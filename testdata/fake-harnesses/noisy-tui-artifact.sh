#!/bin/sh
printf 'MMMMMMMM\n'
printf '\033[?25l\033[2J\033[H'
printf 'MMMMMMMM\n'
sleep 0.1
printf '\033[?25h'
