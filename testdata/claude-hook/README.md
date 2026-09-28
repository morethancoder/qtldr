Real PostToolUse payloads captured from Claude Code 2.1.283 on 2026-09-28:
a scratch project with a `.claude/settings.json` PostToolUse hook
(matcher `Write|Edit|MultiEdit`, command `cat > <file>`), then
`claude -p '<edit a.go, then write b.go>' --allowedTools Edit Write --permission-mode acceptEdits`.
Saved verbatim; tests only replace `tool_input.file_path` at run time.
