trap 'printf "interrupted\n"; exit 130' INT
printf 'ready\n'
while :; do
	IFS= read -r _ || true
done
