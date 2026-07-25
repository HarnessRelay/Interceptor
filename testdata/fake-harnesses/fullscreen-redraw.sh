#!/bin/sh
printf '\033[?1049h'
printf '\033[2J\033[H'
printf 'HarnessRelay fake TUI\n'
printf 'status: drawing frame 1\n'
sleep 0.1
printf '\033[2;1H\033[2Kstatus: drawing frame 2\n'
sleep 0.1
printf '\033[2;1H\033[2Kstatus: done\n'
printf '\033[?1049l'
printf 'fullscreen complete\n'
