# File Management

CraftStack's per-instance file manager (web UI → *Files* tab) lets operators
edit, organize, audit, and roll back changes to files inside an instance's
`work_dir` without shelling into the host.

## Features

| Capability | UI | Shortcut |
|---|---|---|
| Browse / open / edit (Monaco) | Tree click, tab | — |
| New file / folder | Sidebar `+` / `📁` buttons | — |
| Upload (drag-and-drop + multiple) | Drop onto pane | — |
| Rename | Context menu | `F2` |
| Duplicate | Context menu | `Ctrl+D` |
| Multi-select | `Ctrl/Cmd+Click` toggle | — |
| Copy / Cut / Paste | Context menu | `Ctrl+C` / `Ctrl+X` / `Ctrl+V` |
| Move (drag file onto folder) | Tree drag-drop | — |
| Delete (single or batch) | Context menu | `Delete` |
| History | Context menu → *history* | — |
| Rollback (to any prior commit) | History modal → *restore* | — |
| Download | Context menu | — |

All operations are scoped to the instance's `work_dir` — path-traversal
(`../`) and copy-into-self are rejected at the agent.

## Audit log

Every successful file mutation writes an entry to `audit_logs`:

| Field | Value |
|---|---|
| `action` | `write`, `delete`, `rename`, `copy`, `move`, `upload`, `mkdir`, `restore` |
| `target_type` | `file` |
| `target_id` | `<instanceID>:<path>` |
| `target_name` | `<path>` |
| `username` | the logged-in user |
| `detail` | byte count / source path / rollback target SHA |

View from the web UI → *Audit* page (admin/editor/viewer can read).

## Git integration

When `git` is installed on the agent host, CraftStack initializes a local git
repo inside each instance's `work_dir` on first mutation (`git init -b main`,
`.git/` directory inside the work dir) and automatically commits every
successful file change. Missing `git` is not fatal — file operations continue,
only the history / rollback features are unavailable on that host.

- **Author attribution**: each commit's `GIT_AUTHOR_NAME` / `GIT_AUTHOR_EMAIL`
  is set from the acting user's CraftStack account, so `git log` is a genuine
  audit trail.
- **Commit message format**: `[<username>] <verb>: <paths>` (truncated at 200
  chars).
- **Locale**: the agent forces `LANG=C LC_ALL=C` when invoking git so parsing
  is stable across hosts.

### View history

Right-click a file → *history* → modal shows last 50 commits touching that
path (SHA / author / timestamp / message).

REST:
```
GET /api/instances/<id>/files/history?path=<relPath>
→ { success, commits: [{ sha, author, email, timestamp_unix, message }] }
```

### Rollback a file

From the history modal, click **restore** next to any older commit. This:

1. Reads the file content at that commit (`git show <sha>:<path>`).
2. Overwrites the current file with that content.
3. Creates a new rollback commit authored by the requesting user
   (`rollback <path> to <sha>`).
4. Records an audit entry with action `restore`.

REST:
```
POST /api/instances/<id>/files/restore    (admin + editor)
{ "path": "config.yml", "commit_sha": "a1b2c3d" }
→ { success, commit_sha: "<new rollback sha>" }
```

Only hex SHAs that resolve to an existing commit in this repo are accepted;
shell-metacharacter injection and unknown references are rejected at the
agent.

## Limits

- There is no cross-instance file operation. Each instance's `work_dir` is a
  separate namespace (and a separate git repo).
- Git storage is local to the agent host. Nothing is pushed to a remote.
  Operators wanting off-host backup can point a git remote manually inside
  the work dir; CraftStack will not touch remotes or tags.
- `batch-delete` commits the listed paths as a single rollup commit.
