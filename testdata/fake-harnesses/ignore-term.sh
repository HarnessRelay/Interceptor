trap '' TERM
printf 'ready\n'
while :; do
	IFS= read -r _ || true
done
