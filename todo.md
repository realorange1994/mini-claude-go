# Timeout Auto-Background Testing

## Bugs Found

### Bug 1: timeout 参数被 agent_loop 删除，exec tool 收不到
- **位置**: agent_loop.go:2188 `delete(input, "timeout")`
- **原因**: agent_loop 把 timeout 当作 meta-parameter 删除了，但 exec tool 需要这个值
- **修复**: delete 后只对 exec/mcp 重新写入 `input["timeout"] = timeoutMs`（int 类型）

### Bug 2: timeout 单位不一致
- **位置**: agent_loop.go:2177 把 `input["timeout"].(float64)` 当秒用，但 exec schema 定义的是毫秒
- **原因**: 5000ms → agent_loop 当作 5000秒 → clamp 到 600秒
- **修复**: 所有超时统一到毫秒（ms）

### Bug 3: agent_loop 的 context timeout 和 exec/MCP 的 timer timeout 冲突
- **位置**: agent_loop.go:2263 `interruptCtx(context.Background(), timeout)`
- **原因**: agent_loop 用 context.WithTimeout 创建超时 context，5秒后 ctx.Done() 触发，杀掉进程。exec tool 的 timer-based 超时永远没机会触发
- **修复**: 对 exec 和 mcp_call_tool，agent_loop 使用 600000ms (10min) context deadline，让工具自己的 timer 先触发并自动后台

### Bug 4: agent_loop 使用秒而 schema 使用毫秒
- **原因**: toolTimeout 字段是 time.Duration（秒级别），但工具 schema 定义的是毫秒
- **修复**: 将 toolTimeout 改为 toolTimeoutMs (int，单位 ms)，所有超时统一到毫秒

## Test Results

### exec timeout auto-background (one-shot, bypass mode)
- ✅ timeout=5000ms 正确生效，5秒后超时
- ✅ 进程自动转入后台，返回 task ID
- ✅ LLM 能用 task_output 和 task_stop 管理后台任务
- ⚠️ one-shot 模式下进程退出后后台 goroutine 也退出（预期行为，不是 bug）

### MCP timeout auto-background (one-shot, bypass mode)
- ✅ minimax_llm 调用超过 30 秒时自动超时并转入后台
- ✅ 返回 task ID: `mcp-blf2alu39`
- ✅ LLM 能用 task_output 获取后台任务结果
- ✅ MCP 连接保持活动状态（stdin 未被关闭）
- ✅ MCP 后台任务输出文件创建正确

### MCP normal calls
- ✅ minimax_llm 正常调用（<30秒）
- ✅ minimax_video_generation 正常调用
- ✅ list_mcp_tools 正常列出 28 个工具

## Fixed Bugs (committed)
1. timeout 参数删除 → 保留并转换为 int ms（仅对 exec/mcp）
2. timeout 单位统一到毫秒
3. agent_loop context timeout 与 exec/MCP timer timeout 冲突 → exec/MCP 用 600000ms context
4. toolTimeout → toolTimeoutMs 统一毫秒

### Bug 5: 关闭管道杀死 PowerShell 进程，导致 auto-background 失效
- **位置**: exec_tool.go 原来的 `stdout.Close(); stderr.Close()` (超时处理)
- **原因**: 在 Windows 上，关闭 stdout/stderr 管道会导致 PowerShell 进程收到 broken pipe 错误而立即退出，即使进程应该继续在后台运行
- **根因**: Go 的管道在进程退出前关闭会导致操作系统向进程发送 SIGPIPE（Unix）或终止进程（Windows）
- **修复**:
  1. 不再在超时时关闭管道，让进程继续运行
  2. 将 output drain 逻辑从超时 case 移到 `errCh` goroutine 中（等待 `cmd.Wait()` 返回后再 drain）
  3. `errCh` goroutine 在 `cmd.Wait()` 返回后 drain outputCh（此时 reader goroutines 才完成），然后写入输出文件和退出代码
  4. 修复 `onDone` 的 elapsed 计算错误（原来 `time.Since(start)` 在 `start` 之后立即执行，总是0）
- **验证**: ✅ 8行命令（8秒）超时设置为5秒，超时后继续运行3秒，所有输出正确捕获，Duration 准确 (3.457s)，无重复 footer/exit code
