#!/usr/bin/env node
// gsd-hook-version: 1.42.3
// GSD Workflow Guard — PreToolUse hook
// Detects when Claude attempts file edits outside a GSD workflow context
// (no active /gsd- skill or Task subagent) and injects an advisory warning.
//
// This is a SOFT guard — it advises, not blocks. The edit still proceeds.
// The warning nudges Claude to use /gsd:quick or /gsd:fast instead of
// making direct edits that bypass state tracking.
//
// Enable via config: hooks.workflow_guard: true (default: false)
// Only triggers on Write/Edit tool calls to non-.planning/ files.

const fs = require('fs');
const path = require('path');

let input = '';
const stdinTimeout = setTimeout(() => process.exit(0), 3000);
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => input += chunk);
process.stdin.on('end', () => {
  clearTimeout(stdinTimeout);
  try {
    const data = JSON.parse(input);
    const toolName = data.tool_name;

    // Only guard Write and Edit tool calls
    if (toolName !== 'Write' && toolName !== 'Edit') {
      process.exit(0);
    }

    // Check if we're inside a GSD workflow (Task subagent or /gsd- skill)
    // Subagents have a session_id that differs from the parent
    // and typically have a description field set by the orchestrator
    if (data.tool_input?.is_subagent || data.session_type === 'task') {
      process.exit(0);
    }

    // Check the file being edited
    const filePath = data.tool_input?.file_path || data.tool_input?.path || '';

    // Allow edits to .planning/ files (GSD state management).
    // Match .planning as a full path segment so e.g. foo.planning/ is excluded.
    if (/(^|[/\\])\.planning[/\\]/.test(filePath)) {
      process.exit(0);
    }

    // Allow edits to common config/docs files that don't need GSD tracking
    const allowedPatterns = [
      /\.gitignore$/,
      /\.env/,
      /CLAUDE\.md$/,
      /AGENTS\.md$/,
      /GEMINI\.md$/,
      /settings\.json$/,
    ];
    if (allowedPatterns.some(p => p.test(filePath))) {
      process.exit(0);
    }

    // Check if workflow guard is enabled
    const cwd = data.cwd || process.cwd();
    const configPath = path.join(cwd, '.planning', 'config.json');
    if (fs.existsSync(configPath)) {
      try {
        const config = JSON.parse(fs.readFileSync(configPath, 'utf8'));
        if (!config.hooks?.workflow_guard) {
          process.exit(0); // Guard disabled (default)
        }
      } catch (e) {
        process.exit(0);
      }
    } else {
      process.exit(0); // No GSD project — don't guard
    }

    // If we get here: GSD project, guard enabled, file edit outside .planning/.
    // No runtime exposes a reliable "inside a /gsd:* skill or Task subagent"
    // signal in hook payloads, so this advisory can fire on legitimate GSD
    // workflow edits — the wording below makes it self-neutralizing for that
    // case instead of pretending we can detect it.
    // Sanitize the basename: it is attacker-influenced and this guard must
    // not itself become an instruction-injection channel.
    const safeName = path.basename(filePath).replace(/[^\w.-]/g, '_').slice(0, 80);
    const output = {
      hookSpecificOutput: {
        hookEventName: "PreToolUse",
        additionalContext: `⚠️ WORKFLOW ADVISORY: You're editing ${safeName} directly. ` +
          'If you are already working inside a GSD workflow (/gsd:* command or a Task subagent), ' +
          'ignore this advisory and proceed. Otherwise note this edit will not be tracked in ' +
          'STATE.md or produce a SUMMARY.md — consider /gsd:fast for trivial fixes or /gsd:quick ' +
          'for larger changes to maintain project state tracking. ' +
          'If a direct edit is intentional (e.g., the user explicitly asked for it), proceed normally.'
      }
    };

    process.stdout.write(JSON.stringify(output));
  } catch (e) {
    // Silent fail — never block tool execution
    process.exit(0);
  }
});
