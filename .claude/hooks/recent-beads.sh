#!/bin/sh
# After a conversation is summarized, put the board's recent state back in
# front of Claude: the summary can drop work, the beads don't.
since=$(date -v-3d +%Y-%m-%d 2>/dev/null || date -d '3 days ago' +%Y-%m-%d)
echo "# Beads: what this work has done (from the board, not the summary)"
echo
echo "Check here, and with bd show, before saying what has or hasn't been done."
echo
echo "## In progress and open"
bd list --status=in_progress,open 2>/dev/null
echo
echo "## Closed since $since"
bd list --closed-after "$since" --status=closed --sort=closed 2>/dev/null | head -40
