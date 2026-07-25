printf 'ready\n'
trap 'stty size' WINCH
while IFS= read -r line; do
	if [ "$line" = "size" ]; then
		stty size
	fi
done
