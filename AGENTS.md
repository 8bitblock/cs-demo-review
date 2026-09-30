# Project instructions

- Any time project or session coding is done, append additions and fixes as a dot-pointed list in `CHANGELOG.md`, and include a concise dot-pointed summary in the final response.
- This is a Windows Electron/React application with Go parser workers. Use Node.js 22.12+ and Go 1.25+.
- For a fresh checkout or worktree, run `npm run setup:worktree`. The ChatGPT/Codex local environment runs the same setup automatically when selected for a new worktree.
- Run commands from the active checkout root. Keep its `node_modules`, worker binaries, build output, and `.tmp` directories local to that checkout.
- Use `npm run dev` for desktop development. Source launches keep their library and Electron profile under the checkout's ignored `.tmp` directory. Packaged builds retain their normal user profile.
- Use `npm run typecheck` and `npm test` for TypeScript changes; use `npm run test:worker` for Go changes. For startup/build changes, also verify `npm run build:worker` and `npm run build:app`.
- Keep local demo recordings, libraries, game assets, tool installations, and release binaries out of Git. Optional map extraction uses `node scripts/setup-tools.mjs --extractor`.
- Create feature branches with the `codex/` prefix when a branch is needed. See `docs/WORKTREES.md` for the app workflow.
