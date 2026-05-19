# OAuth & Authentication

> OAuth, authentication

## Sections Included
- [###] Line 10243-10260 -- ### 46.4 Authentication / OAuth

---

## Content

### 46.4 Authentication / OAuth

| # | Aspect | Go | Upstream (`auth.ts`) | Type |
|---|--------|----|---------------------|------|
| 1 | Multi-source auth resolution | env only: ANTHROPIC_API_KEY → ANTHROPIC_AUTH_TOKEN (main.go:67-78) | 5 sources with precedence: env → file descriptor → apiKeyHelper → keychain → config (auth.ts:227-349) | 简化 |
| 2 | OAuth 2.0 PKCE flow | Not implemented — API key only | `OAuthService`, code verifier/challenge (services/oauth/index.ts) | 缺失 |
| 3 | OAuth token storage (keychain / credentials file) | Not implemented | `saveOAuthTokensIfNeeded`, `getClaudeAIOAuthTokens` (auth.ts:1189-1296) | 缺失 |
| 4 | OAuth token refresh with lock-based dedup | Not implemented | `checkAndRefreshOAuthTokenIfNeeded` (auth.ts:1423-1558) | 缺失 |
| 5 | macOS Keychain integration | Not implemented | `getApiKeyFromConfigOrMacOSKeychain`, `saveApiKey` (auth.ts:1047-1187) | 缺失 |
| 6 | AWS auth refresh (STS) | Not implemented | `awsAuthRefresh` + `awsCredentialExport` (auth.ts:604-808) | 缺失 |
| 7 | GCP auth refresh | Not implemented | `gcpAuthRefresh` + credential check (auth.ts:813-1010) | 缺失 |
| 8 | apiKeyHelper (exec helper with SWR cache) | Not implemented | `apiKeyHelper` settings with TTL, trust guard (auth.ts:356-603) | 缺失 |
| 9 | Subscription type detection | Not implemented | `getSubscriptionType`: max/pro/enterprise/team (auth.ts:1658-1673) | 缺失 |
| 10 | Rate limit tier detection | Not implemented | Rate limit tier from auth (auth.ts:1698-1708) | 缺失 |
| 11 | Auth token source tracking | Not implemented | `getAuthTokenSource`: env/FD/helper/keychain/oauth (auth.ts:154-207) | 缺失 |
| 12 | 3P auth detection (Bedrock/Vertex/Foundry) | Not implemented | `isAnthropicAuthEnabled` (auth.ts:100-150) | 缺失 |
| 13 | Auth Bearer header construction | Simple `"Bearer "+apiKey` (agent_loop.go:367) | OAuth-aware, `x-api-key` vs `Authorization` | 简化 |


---

