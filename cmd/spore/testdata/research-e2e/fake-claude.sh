#!/bin/sh
while IFS= read -r line; do
	case $line in
	"write out.txt: "*) printf '%s\n' "${line#write out.txt: }" > out.txt ;;
	esac
done < .claude/skills/e2e/SKILL.md
printf '%s\n' '{"type":"result","result":"ok","total_cost_usd":0}'
