#!/usr/bin/env python3
"""Fix raw newlines inside Go string literals in agent_loop.go"""
import sys

with open('agent_loop.go', 'r', encoding='utf-8') as f:
    content = f.read()

# Find specific broken patterns and fix them
# Pattern: summaryContent += "\n\n## Custom instructions...\n" + preCompactInst
# where \n are real newlines

# We know the exact broken text from the line analysis
# In trySMCompact around line 3945:
#   \t\tsummaryContent += "
#
#   ## Custom instructions for this compaction:
#   " + preCompactInst
broken1 = '\t\tsummaryContent += "\n\n## Custom instructions for this compaction:\n" + preCompactInst'
fixed1 = '\t\tsummaryContent += "\\n\\n## Custom instructions for this compaction:\\n" + preCompactInst'

# In tryLLMCompaction around line 4062:
#   \t\t\ta.context.AddSummary("
#
#   ## Custom instructions for this compaction:
#   " + preCompactInst)
broken2 = '\t\t\ta.context.AddSummary("\n\n## Custom instructions for this compaction:\n" + preCompactInst)'
fixed2 = '\t\t\ta.context.AddSummary("\\n\\n## Custom instructions for this compaction:\\n" + preCompactInst)'

changes = 0
if broken1 in content:
    content = content.replace(broken1, fixed1, 1)
    changes += 1
    print('Fixed pattern 1 (trySMCompact)')
else:
    print('Pattern 1 not found')

if broken2 in content:
    content = content.replace(broken2, fixed2, 1)
    changes += 1
    print('Fixed pattern 2 (tryLLMCompaction)')
else:
    print('Pattern 2 not found')

if changes > 0:
    with open('agent_loop.go', 'w', encoding='utf-8') as f:
        f.write(content)
    print(f'Wrote {changes} fixes')
else:
    # Debug: find what's actually there
    for i, line in enumerate(content.split('\n')):
        if 'Custom instructions' in line:
            print(f'Line {i+1}: {repr(line[:120])}')
