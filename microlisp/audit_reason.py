import re

with open('ffi_gen/lisp_gen.go', encoding='utf-8', errors='replace') as f:
    content = f.read()

# Extract incompatibleParamPatterns
pattern_match = re.search(r'var incompatibleParamPatterns = \[\]string\{(.*?)\}', content, re.DOTALL)
incompatible = []
if pattern_match:
    for m in re.finditer(r'"([^"]+)"', pattern_match.group(1)):
        incompatible.append(m.group(1))

# Extract skipFuncs
skip_match = re.search(r'var skipFuncs = map\[string\]bool\{(.*?)\}', content, re.DOTALL)
skip_entries = set()
if skip_match:
    for m in re.finditer(r'"([^"]+)":\s*true', skip_match.group(1)):
        skip_entries.add(m.group(1))

# Read the missing functions NOT in skipFuncs
with open('audit.txt', encoding='utf-8', errors='replace') as f:
    lines = f.readlines()

in_func = False
func_entries = []
for line in lines:
    line = line.strip()
    if line == '--- FUNC (340) ---':
        in_func = True
        continue
    if line.startswith('--- ') and in_func:
        in_func = False
        continue
    if in_func and line:
        func_entries.append(line)

not_skipped = [e for e in func_entries if e not in skip_entries]

# For each function, find why it was skipped
# We need to look at the Go signature. Let's just group by the incompatible param pattern
import subprocess
import os

# Run Go to get the signatures
print(f"Analyzing {len(not_skipped)} missing functions...\n")

for fn in sorted(not_skipped):
    parts = fn.rsplit('.', 1)
    if len(parts) != 2:
        print(f"  {fn} -> can't parse")
        continue
    pkg, name = parts
    
    # Check incompatible params
    found_reason = None
    for pattern in incompatible:
        # We'd need the actual Go signature to check this
        pass
    
    print(f"  {fn}")
    
