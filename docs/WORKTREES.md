# Worktrees in ChatGPT / Codex

The `demo viewer` project is a Git repository backed by `8bitblock/cs-demo-review` on GitHub. Its default branch is `master`.

## Start an independent chat

1. In the ChatGPT desktop app, select **Codex**, then start a new chat in **demo viewer**.
2. Select **Worktree** below the composer and choose **master** as the starting branch.
3. Select the **CS Demo Review** local environment and submit the task. Wait for its setup to finish before building or running the app.
4. Use **Run desktop app** in the environment actions, or run `npm run dev` in the chat's terminal.

The environment lives in `.codex/environments/environment.toml`. If it is not listed, reopen the project and check **Settings > Local environments**, selecting this repository's configuration. Local environments are available in Codex in the desktop app.

Worktree chats initially use a detached HEAD. To publish work, use **Create branch here** with a name such as `codex/replay-controls`, commit, push, and open a pull request. Use **Hand off** to move a chat between its worktree and the local checkout. Git cannot check out the same branch in two places simultaneously.

## What setup does

`npm run setup:worktree` checks Node.js and Go, installs locked npm dependencies with `npm ci`, checks TypeScript, and builds the Go workers and Electron/React app. It stops on a failed step. Run it manually for an ordinary clone or manually created Git worktree. Close any running app from that checkout before rerunning setup, since dependencies and binaries are rebuilt.

Windows with Node.js 22.12+ is required. Setup uses `GO_BINARY`, a checkout-local portable Go installation, or Go on PATH, in that order. If none exists, it installs the existing checksum-pinned portable Go toolchain through `scripts/setup-tools.mjs`. Go 1.25+ is required. Fresh installs need network access to fetch dependencies.

For local app-managed worktrees, `.worktreeinclude` copies an existing portable Go installation and optional Source2Viewer installation. Other ignored files are not included by this project configuration. Ordinary Git worktrees do not perform this copy. Install the optional map extractor with the environment action or `node scripts/setup-tools.mjs --extractor` when needed.

Each checkout has separate dependencies, builds, Electron profile, library, settings, and clips. Source app data lives in `.tmp/electron-user-data`, and the dev server selects an available loopback port so multiple checkouts can run together. An explicit `CS_DEMO_REVIEW_DATA_DIR` overrides the library location; use a different directory per concurrent app. The packaged app keeps its existing user-data location.

Bootstrap builds do not regenerate the tracked third-party license bundle. Release packaging still uses `npm run package`, which runs the full license-generation build.

## Verification

```powershell
npm run typecheck
npm test
npm run test:worker
```

Real-demo integration checks still need locally supplied `.dem` files. They are intentionally ignored by Git.

Official references: [Git worktrees](https://learn.chatgpt.com/docs/environments/git-worktrees) and [local environments](https://learn.chatgpt.com/docs/environments/local-environment).
