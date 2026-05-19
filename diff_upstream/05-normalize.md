# Message Normalization

> Message and content normalization

## Sections Included
- [##] Line 145-202 -- ## 2. Message Normalization

---

## Content

## 2. Message Normalization

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\normalize.go` (217 lines)
- **Upstream**: `E:\Git\claude-code-upstream\src\utils\messages.ts` (`normalizeMessagesForAPI` at line 2018, ~500 lines of normalization logic)

### 2.1 Scope Comparison

| Capability | Go | Upstream TS |
|-----------|-----|-------------|
| JSON key sorting | Yes (line 139-155, recursive) | No (not needed -- SDK handles) |
| Tool result whitespace | Yes (line 175-201) | Yes (via various content normalization) |
| Role alternation enforcement | **No** | Yes -- `mergeAdjacentUserMessages` at lines 2488-2499 |
| Tool pairing validation | **No** | Extensive -- `ensureToolResultPairing` at line 5212+ |
| Empty message handling | **No** | Yes -- `ensureNonEmptyAssistantContent`, `filterWhitespaceOnlyAssistantMessages` |
| Image/media validation | **No** | Yes -- `validateImagesForAPI` at line 2401 |
| Tool input normalization | **No** | Yes -- `normalizeContentFromAPI` at line 2688 |
| Orphaned tool result stripping | **No** | Yes -- in `ensureToolResultPairing` at line 5240+ |
| Thinking block filtering | **No** | Yes -- `filterOrphanedThinkingOnlyMessages` (line 5070), `filterTrailingThinkingFromLastAssistant` |
| Message ID tag injection | **No** | Yes -- `[id:...]` snip tool tags at lines 2385-2398 |
| Attachment handling | **No** | Yes -- attachment messages normalized at lines 2303-2324 |
| Virtual message stripping | **No** | Yes -- `.filter(m => !m.isVirtual)` at line 2029 |
| Content block type stripping | **No** | Yes -- strips document/image blocks on error (lines 2033-2083) |
| Tool reference block handling | **No** | Yes -- strip/relocate tool_reference siblings (lines 2188-2213, 2335-2339) |
| System message conversion | **No** | Yes -- converts local_command system to user (lines 2107-2121) |
| System reminder sibling smooshing | **No** | Yes -- `smooshSystemReminderSiblings` at line 2371 |
| Error tool result sanitization | **No** | Yes -- `sanitizeErrorToolResultContent` at line 2377 |

### 2.2 Normalization Pipeline

**Go** (single-pass, targeted):
```
NormalizeAPIMessages -> normalizeMessage -> normalizeAssistantMessage / normalizeUserMessage
  -> sortMapKeys (tool_use input JSON)
  -> normalizeWhitespace (tool_result text)
```

**Upstream** (multi-pass pipeline in normalizeMessagesForAPI, lines 2328-2403):
```
reorderAttachmentsForAPI
  -> filter virtual messages
  -> strip targets (images/docs on error)
  -> tool_reference handling
  -> merge consecutive user/assistant messages
  -> filterOrphanedThinkingOnlyMessages
  -> filterTrailingThinkingFromLastAssistant
  -> filterWhitespaceOnlyAssistantMessages
  -> ensureNonEmptyAssistantContent
  -> smooshSystemReminderSiblings
  -> sanitizeErrorToolResultContent
  -> append message ID tags
  -> validateImagesForAPI
```

**Gap**: Go's normalization is narrowly scoped to cache-friendly JSON/whitespace normalization. It lacks the extensive role alternation, tool pairing, content filtering, and validation pipeline that upstream runs before every API call.

---


---

