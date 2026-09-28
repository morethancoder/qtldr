// Package hooks installs qtldr into Claude Code (.claude/settings.json) and
// git (pre-push). The merge logic is pure; Install* do the file I/O.
package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Claude Code hook settings (verified against a real payload capture and the
// hooks docs; see docs/decisions.md #10).
const (
	ClaudeEvent   = "PostToolUse"
	ClaudeMatcher = "Write|Edit|MultiEdit"
	ClaudeCommand = "qtldr check --fast --quiet --files-from-stdin"
	ClaudeTimeout = 30
)

// Snippet is what `hook install` prints for CLAUDE.md (PLAN.md §9.3).
const Snippet = `- This repo uses qtldr. Before changing a function, run ` + "`qtldr show <id>`" + ` (or MCP get_node) for its CRAP, coverage and surviving mutants.
- When the user says "fix what I'm looking at", call get_focus first.
- After editing, run ` + "`qtldr check --changed`" + `. Targets: CRAP ≤ 8, coverage ≥ 80%, mutation ≥ 70%.
`

// GitMarker identifies a pre-push hook written by qtldr.
const GitMarker = "# qtldr-hook"

type handler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

type group struct {
	Matcher string    `json:"matcher"`
	Hooks   []handler `json:"hooks"`
}

// MergeClaudeSettings adds the qtldr PostToolUse hook to settings JSON
// (empty input means no file yet). Existing keys keep their order and are
// never removed; an identical entry is not duplicated. changed is false when
// the hook was already there.
func MergeClaudeSettings(existing []byte) (out []byte, changed bool, err error) {
	root := object{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, false, fmt.Errorf("settings.json is not a JSON object: %w", err)
		}
	}
	hooksObj := object{}
	if raw, ok := root.get("hooks"); ok {
		if err := json.Unmarshal(raw, &hooksObj); err != nil {
			return nil, false, fmt.Errorf(`settings.json "hooks" is not an object: %w`, err)
		}
	}
	var groups []json.RawMessage
	if raw, ok := hooksObj.get(ClaudeEvent); ok {
		if err := json.Unmarshal(raw, &groups); err != nil {
			return nil, false, fmt.Errorf(`settings.json "hooks.%s" is not an array: %w`, ClaudeEvent, err)
		}
	}
	if installed(groups) {
		return existing, false, nil
	}
	entry, _ := json.Marshal(group{Matcher: ClaudeMatcher, Hooks: []handler{{Type: "command", Command: ClaudeCommand, Timeout: ClaudeTimeout}}})
	groups = append(groups, entry)
	hooksObj.set(ClaudeEvent, mustMarshal(groups))
	root.set("hooks", mustMarshal(hooksObj))
	out, err = json.MarshalIndent(root, "", "  ")
	return append(out, '\n'), true, err
}

func installed(groups []json.RawMessage) bool {
	for _, raw := range groups {
		var g group
		if json.Unmarshal(raw, &g) != nil || g.Matcher != ClaudeMatcher {
			continue
		}
		for _, h := range g.Hooks {
			if h.Command == ClaudeCommand {
				return true
			}
		}
	}
	return false
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// InstallClaude merges the hook into <dir>/.claude/settings.json.
func InstallClaude(dir string) (path string, changed bool, err error) {
	path = filepath.Join(dir, ".claude", "settings.json")
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return path, false, fmt.Errorf("read %s: %w", path, err)
	}
	out, changed, err := MergeClaudeSettings(b)
	if err != nil {
		return path, false, fmt.Errorf("%s: %w", path, err)
	}
	if !changed {
		return path, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, false, err
	}
	return path, true, os.WriteFile(path, out, 0o644)
}

// GitHookScript is the pre-push hook body. moduleDir is the module root
// relative to the repository top level ("" when they are the same).
func GitHookScript(moduleDir string) string {
	cd := `cd "$(git rev-parse --show-toplevel)"`
	if moduleDir != "" {
		cd = `cd "$(git rev-parse --show-toplevel)/` + strings.TrimSuffix(moduleDir, "/") + `"`
	}
	return "#!/bin/sh\n" + GitMarker + "\n" +
		"# Installed by `qtldr hook install --git`: blocks the push when changed code\n" +
		"# breaches the thresholds in .qtldr.toml. Skip once with `git push --no-verify`.\n" +
		cd + " || exit 1\n" +
		"exec qtldr check --changed\n"
}

// ErrForeignHook means a pre-push hook exists that qtldr did not write.
var ErrForeignHook = errors.New("a pre-push hook without the qtldr marker already exists")

// InstallGit writes the pre-push hook at path, refusing to replace a hook
// that lacks GitMarker.
func InstallGit(path, moduleDir string) error {
	b, err := os.ReadFile(path)
	switch {
	case err == nil && !bytes.Contains(b, []byte(GitMarker)):
		return fmt.Errorf("%w at %s; add `qtldr check --changed` to it yourself, or remove it and re-run", ErrForeignHook, path)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(GitHookScript(moduleDir)), 0o755)
}

// MCPServer is the .mcp.json entry that starts `qtldr mcp` (Claude Code
// project MCP config: {"mcpServers": {name: {type, command, args}}}).
var MCPServer = json.RawMessage(`{"type":"stdio","command":"qtldr","args":["mcp"]}`)

// MergeMCPConfig adds the qtldr server to .mcp.json content (empty = no file),
// keeping other servers and key order. An existing "qtldr" entry is kept.
func MergeMCPConfig(existing []byte) (out []byte, changed bool, err error) {
	root := object{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, false, fmt.Errorf(".mcp.json is not a JSON object: %w", err)
		}
	}
	servers := object{}
	if raw, ok := root.get("mcpServers"); ok {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, false, fmt.Errorf(`.mcp.json "mcpServers" is not an object: %w`, err)
		}
	}
	if _, ok := servers.get("qtldr"); ok {
		return existing, false, nil
	}
	servers.set("qtldr", MCPServer)
	root.set("mcpServers", mustMarshal(servers))
	out, err = json.MarshalIndent(root, "", "  ")
	return append(out, '\n'), true, err
}

// InstallMCP merges the qtldr server into <dir>/.mcp.json.
func InstallMCP(dir string) (path string, changed bool, err error) {
	path = filepath.Join(dir, ".mcp.json")
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return path, false, fmt.Errorf("read %s: %w", path, err)
	}
	out, changed, err := MergeMCPConfig(b)
	if err != nil || !changed {
		return path, false, err
	}
	return path, true, os.WriteFile(path, out, 0o644)
}
