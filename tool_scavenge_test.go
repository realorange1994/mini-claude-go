package main

import (
	"reflect"
	"testing"
)

func TestScavengeToolCallBlock_UserFormat(t *testing.T) {
	// The exact format from user logs
	input := `[TOOL_CALL]
{tool => "read_file", args => {
  --file_path "E:\\Git\\nanoclaude_minimal\\nanoclaude_minimal.go"
}}
[/TOOL_CALL]`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0]["name"] != "read_file" {
		t.Errorf("expected name=read_file, got %v", calls[0]["name"])
	}

	args := calls[0]["input"].(map[string]any)
	// Input has literal \\ (double backslash), so output should also have double backslash
	expected := `E:\\Git\\nanoclaude_minimal\\nanoclaude_minimal.go`
	if args["file_path"] != expected {
		t.Errorf("expected file_path=%s, got %v", expected, args["file_path"])
	}
}

func TestScavengeToolCallBlock_SingleLine(t *testing.T) {
	input := `[TOOL_CALL]{tool => "bash", args => { --command "ls -la" }}[/TOOL_CALL]`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0]["name"] != "bash" {
		t.Errorf("expected name=bash, got %v", calls[0]["name"])
	}

	args := calls[0]["input"].(map[string]any)
	if args["command"] != "ls -la" {
		t.Errorf("expected command=ls -la, got %v", args["command"])
	}
}

func TestScavengeToolCallBlock_Lowercase(t *testing.T) {
	input := `[tool_call]{tool => "write_file", args => { --path "/tmp/test.txt" }}[/tool_call]`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0]["name"] != "write_file" {
		t.Errorf("expected name=write_file, got %v", calls[0]["name"])
	}

	args := calls[0]["input"].(map[string]any)
	if args["path"] != "/tmp/test.txt" {
		t.Errorf("expected path=/tmp/test.txt, got %v", args["path"])
	}
}

func TestScavengeToolCallBlock_MultipleCalls(t *testing.T) {
	input := `[TOOL_CALL]{tool => "read_file", args => { --path "a.txt" }}[/TOOL_CALL]
some text in between
[TOOL_CALL]{tool => "bash", args => { --command "pwd" }}[/TOOL_CALL]`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}

	if calls[0]["name"] != "read_file" {
		t.Errorf("expected first name=read_file, got %v", calls[0]["name"])
	}
	if calls[1]["name"] != "bash" {
		t.Errorf("expected second name=bash, got %v", calls[1]["name"])
	}
}

func TestScavengeToolCallBlock_MultipleArgs(t *testing.T) {
	input := `[TOOL_CALL]
{tool => "file_edit", args => {
  --file_path "main.go"
  --old_string "func foo()"
  --new_string "func bar()"
}}
[/TOOL_CALL]`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	args := calls[0]["input"].(map[string]any)
	if args["file_path"] != "main.go" {
		t.Errorf("expected file_path=main.go, got %v", args["file_path"])
	}
	if args["old_string"] != "func foo()" {
		t.Errorf("expected old_string=func foo(), got %v", args["old_string"])
	}
	if args["new_string"] != "func bar()" {
		t.Errorf("expected new_string=func bar(), got %v", args["new_string"])
	}
}

func TestScavengeToolCallBlock_NoArgs(t *testing.T) {
	input := `[TOOL_CALL]{tool => "list_dir"}[/TOOL_CALL]`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0]["name"] != "list_dir" {
		t.Errorf("expected name=list_dir, got %v", calls[0]["name"])
	}

	args := calls[0]["input"].(map[string]any)
	if len(args) != 0 {
		t.Errorf("expected empty args, got %v", args)
	}
}

func TestScavengeToolCallBlock_SingleQuotes(t *testing.T) {
	input := `[TOOL_CALL]{tool => 'bash', args => { --command 'echo hello' }}[/TOOL_CALL]`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0]["name"] != "bash" {
		t.Errorf("expected name=bash, got %v", calls[0]["name"])
	}

	args := calls[0]["input"].(map[string]any)
	if args["command"] != "echo hello" {
		t.Errorf("expected command=echo hello, got %v", args["command"])
	}
}

func TestScavengeToolCallBlock_NumberArgs(t *testing.T) {
	input := `[TOOL_CALL]{tool => "set_config", args => { --timeout 30 --retries 3 }}[/TOOL_CALL]`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	args := calls[0]["input"].(map[string]any)
	if args["timeout"] != "30" {
		t.Errorf("expected timeout=30, got %v", args["timeout"])
	}
	if args["retries"] != "3" {
		t.Errorf("expected retries=3, got %v", args["retries"])
	}
}

func TestScavengeToolCallBlock_NestedBraces(t *testing.T) {
	input := `[TOOL_CALL]{tool => "eval", args => { --code "func() { return 42 }" }}[/TOOL_CALL]`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	args := calls[0]["input"].(map[string]any)
	if args["code"] != "func() { return 42 }" {
		t.Errorf("expected code to handle nested braces, got %v", args["code"])
	}
}

func TestScavengeToolCallBlock_MissingClosingTag(t *testing.T) {
	input := `[TOOL_CALL]{tool => "bash", args => { --command "ls" }}`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for unclosed block, got %d", len(calls))
	}
}

func TestScavengeToolCallBlock_NoToolKeyword(t *testing.T) {
	input := `[TOOL_CALL]{ args => { --key "value" }}[/TOOL_CALL]`

	calls := scavengeToolCallBlock(input)
	// No tool => found, should return empty
	if len(calls) != 0 {
		t.Errorf("expected 0 calls (no tool keyword), got %d", len(calls))
	}
}

func TestScavengeDSML_MultipleParams(t *testing.T) {
	input := `<|DSML|invoke name="file_edit">
<|DSML|parameter name="path">main.go</|DSML|parameter>
<|DSML|parameter name="old">func a()</|DSML|parameter>
<|DSML|parameter name="new">func b()</|DSML|parameter>
</|DSML|invoke>`

	calls := scavengeDSML(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	args := calls[0]["input"].(map[string]any)
	if args["path"] != "main.go" {
		t.Errorf("expected path=main.go, got %v", args["path"])
	}
	if args["old"] != "func a()" {
		t.Errorf("expected old=func a(), got %v", args["old"])
	}
	if args["new"] != "func b()" {
		t.Errorf("expected new=func b(), got %v", args["new"])
	}
}

func TestScavengeDSML_StringFalse(t *testing.T) {
	input := `<|DSML|invoke name="parse_json">
<|DSML|parameter name="raw" string="false">{"key": "value"}</|DSML|parameter>
</|DSML|invoke>`

	calls := scavengeDSML(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	args := calls[0]["input"].(map[string]any)
	// string="false" attempts JSON parse → returns map
	nested, ok := args["raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected raw to be parsed as map, got %T", args["raw"])
	}
	if nested["key"] != "value" {
		t.Errorf("expected nested key=value, got %v", nested["key"])
	}
}

func TestScavengeDSML_StringTrue(t *testing.T) {
	input := `<|DSML|invoke name="log">
<|DSML|parameter name="message" string="true">{"not json"}</|DSML|parameter>
</|DSML|invoke>`

	calls := scavengeDSML(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	args := calls[0]["input"].(map[string]any)
	// string="true" should keep as literal string (not JSON parse)
	if args["message"] != `{"not json"}` {
		t.Errorf("expected message as literal string, got %v", args["message"])
	}
}

func TestScavengeDSML_NoParams(t *testing.T) {
	input := `<|DSML|invoke name="list_dir"></|DSML|invoke>`

	calls := scavengeDSML(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0]["name"] != "list_dir" {
		t.Errorf("expected name=list_dir, got %v", calls[0]["name"])
	}

	args := calls[0]["input"].(map[string]any)
	if len(args) != 0 {
		t.Errorf("expected empty args, got %v", args)
	}
}

func TestScavengeDSML_MultipleInvokes(t *testing.T) {
	input := `<|DSML|invoke name="read_file">
<|DSML|parameter name="path">a.txt</|DSML|parameter>
</|DSML|invoke>
some text
<|DSML|invoke name="bash">
<|DSML|parameter name="command">cat b.txt</|DSML|parameter>
</|DSML|invoke>`

	calls := scavengeDSML(input)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}

	if calls[0]["name"] != "read_file" {
		t.Errorf("expected first=read_file, got %v", calls[0]["name"])
	}
	if calls[1]["name"] != "bash" {
		t.Errorf("expected second=bash, got %v", calls[1]["name"])
	}
}

func TestScavengeRawJSON_MultipleArgs(t *testing.T) {
	input := `{ "name": "file_edit", "arguments": {"path": "main.go", "old": "a", "new": "b"} }`

	calls := scavengeRawJSON(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	args := calls[0]["input"].(map[string]any)
	if args["path"] != "main.go" {
		t.Errorf("expected path=main.go, got %v", args["path"])
	}
	if args["old"] != "a" {
		t.Errorf("expected old=a, got %v", args["old"])
	}
	if args["new"] != "b" {
		t.Errorf("expected new=b, got %v", args["new"])
	}
}

func TestScavengeRawJSON_EmptyArguments(t *testing.T) {
	input := `{ "name": "list_dir", "arguments": {} }`

	calls := scavengeRawJSON(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0]["name"] != "list_dir" {
		t.Errorf("expected name=list_dir, got %v", calls[0]["name"])
	}

	args := calls[0]["input"].(map[string]any)
	if len(args) != 0 {
		t.Errorf("expected empty args, got %v", args)
	}
}

func TestScavengeRawJSON_MixedWithText(t *testing.T) {
	input := `Let me read the file: { "name": "read_file", "arguments": {"path": "config.json"} }
Now let me run a command: { "name": "bash", "arguments": {"command": "ls"} }`

	calls := scavengeRawJSON(input)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
}

func TestScavengeRawJSON_Pattern1And2Coexist(t *testing.T) {
	input := `{ "name": "simple_tool", "arguments": {"a": "1"} }
{ "type": "function", "function": { "name": "openai_tool", "arguments": {"b": "2"} } }`

	calls := scavengeRawJSON(input)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls (one of each pattern), got %d", len(calls))
	}

	names := make(map[string]bool)
	for _, c := range calls {
		names[c["name"].(string)] = true
	}
	if !names["simple_tool"] || !names["openai_tool"] {
		t.Errorf("expected both patterns, got names %v", names)
	}
}

func TestScavengeToolCalls_CrossPatternDedup(t *testing.T) {
	// Same logical call in different formats should only appear once
	input := `[TOOL_CALL]{tool => "bash", args => { --command "ls" }}[/TOOL_CALL]
{ "name": "bash", "arguments": {"command": "ls"} }`

	calls := ScavengeToolCalls([]string{input})
	if len(calls) != 1 {
		t.Errorf("expected 1 call (cross-pattern dedup), got %d", len(calls))
	}
}

func TestScavengeToolCalls_MultipleTextParts(t *testing.T) {
	parts := []string{
		`<|DSML|invoke name="dsml_call"><|DSML|parameter name="x">1</|DSML|parameter></|DSML|invoke>`,
		`thinking...`,
		`[TOOL_CALL]{tool => "tool_call", args => { --key "val" }}[/TOOL_CALL]`,
		`more thinking...`,
		`{ "name": "json_call", "arguments": {"y": "2"} }`,
	}

	calls := ScavengeToolCalls(parts)
	if len(calls) != 3 {
		t.Fatalf("expected 3 calls from multiple text parts, got %d", len(calls))
	}

	names := make(map[string]bool)
	for _, c := range calls {
		names[c["name"].(string)] = true
	}

	expected := map[string]bool{
		"dsml_call":   true,
		"tool_call":   true,
		"json_call": true,
	}
	if !reflect.DeepEqual(names, expected) {
		t.Errorf("expected names %v, got %v", expected, names)
	}
}

func TestScavengeToolCallBlock_NoMatch(t *testing.T) {
	input := `just some normal text without any tool calls`

	calls := scavengeToolCallBlock(input)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestScavengeRawJSON_Pattern1(t *testing.T) {
	input := `Here is a tool call: { "name": "read_file", "arguments": {"path": "/tmp/test.txt"} }`

	calls := scavengeRawJSON(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0]["name"] != "read_file" {
		t.Errorf("expected name=read_file, got %v", calls[0]["name"])
	}

	args := calls[0]["input"].(map[string]any)
	if args["path"] != "/tmp/test.txt" {
		t.Errorf("expected path=/tmp/test.txt, got %v", args["path"])
	}
}

func TestScavengeRawJSON_Pattern2(t *testing.T) {
	input := `{ "type": "function", "function": { "name": "bash", "arguments": {"command": "echo hello"} } }`

	calls := scavengeRawJSON(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0]["name"] != "bash" {
		t.Errorf("expected name=bash, got %v", calls[0]["name"])
	}

	args := calls[0]["input"].(map[string]any)
	if args["command"] != "echo hello" {
		t.Errorf("expected command=echo hello, got %v", args["command"])
	}
}

func TestScavengeDSML(t *testing.T) {
	input := `<|DSML|invoke name="read_file">
<|DSML|parameter name="path">/tmp/test.txt</|DSML|parameter>
</|DSML|invoke>`

	calls := scavengeDSML(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	if calls[0]["name"] != "read_file" {
		t.Errorf("expected name=read_file, got %v", calls[0]["name"])
	}

	args := calls[0]["input"].(map[string]any)
	if args["path"] != "/tmp/test.txt" {
		t.Errorf("expected path=/tmp/test.txt, got %v", args["path"])
	}
}

func TestScavengeToolCalls_AllPatterns(t *testing.T) {
	// All four patterns in one text
	input := `Some thinking text here...

<|DSML|invoke name="dsml_tool">
<|DSML|parameter name="arg1">val1</|DSML|parameter>
</|DSML|invoke>

[TOOL_CALL]{tool => "tool_call_tool", args => { --key "value" }}[/TOOL_CALL]

{ "name": "raw_json_tool", "arguments": {"x": "y"} }

{ "type": "function", "function": { "name": "openai_tool", "arguments": {"z": "w"} } }`

	calls := ScavengeToolCalls([]string{input})

	// Should find all 4 patterns
	names := make(map[string]bool)
	for _, c := range calls {
		names[c["name"].(string)] = true
	}

	expected := map[string]bool{
		"dsml_tool":     true,
		"tool_call_tool": true,
		"raw_json_tool": true,
		"openai_tool":   true,
	}

	if !reflect.DeepEqual(names, expected) {
		t.Errorf("expected names %v, got %v", expected, names)
	}
}

func TestScavengeToolCalls_Dedup(t *testing.T) {
	// Same call twice should only appear once
	input := `[TOOL_CALL]{tool => "bash", args => { --command "ls" }}[/TOOL_CALL]
[TOOL_CALL]{tool => "bash", args => { --command "ls" }}[/TOOL_CALL]`

	calls := ScavengeToolCalls([]string{input})
	if len(calls) != 1 {
		t.Errorf("expected 1 call (dedup), got %d", len(calls))
	}
}

func TestScavengeToolCalls_EmptyInput(t *testing.T) {
	calls := ScavengeToolCalls(nil)
	if calls != nil {
		t.Errorf("expected nil for nil input, got %v", calls)
	}

	calls = ScavengeToolCalls([]string{})
	if calls != nil {
		t.Errorf("expected nil for empty input, got %v", calls)
	}
}
