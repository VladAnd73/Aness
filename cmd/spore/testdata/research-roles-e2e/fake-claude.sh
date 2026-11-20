#!/bin/sh
# Stands in for claude: the last argument is the prompt. A grader
# prompt holds <repro>; anything else is the role agent.
for a; do prompt=$a; done
case $prompt in
*"<repro>"*)
	trigger=$(printf '%s\n' "$prompt" | sed -n 's/^trigger: //p' | head -n 1)
	hit=$(printf '%s\n' "$prompt" | sed -n '/^<output>$/,/^<\/output>$/p' | grep -F -- "$trigger" | head -n 1)
	if [ -n "$trigger" ] && [ -n "$hit" ]; then
		printf 'QUOTE: %s\nVERDICT: CAUGHT\n' "$hit"
	else
		printf 'VERDICT: MISSED\n'
	fi
	;;
*)
	mkdir -p "$SPORE_TASK_DIR/intake"
	printf '%s\n' "$prompt" | sed -n 's/^cover: /- scenario: /p' > "$SPORE_TASK_DIR/intake/plan.md"
	printf '{"summary":"plan written","plan_path":"intake/plan.md"}\n' > "$SPORE_TASK_DIR/intake/round-1.json"
	printf '%s\n' '{"type":"result","result":"ok","total_cost_usd":0}'
	;;
esac
