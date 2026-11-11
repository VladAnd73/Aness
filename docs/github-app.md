# GitHub App (`spore gh`)

`spore gh` lets a host act on GitHub as the project's GitHub App:
pushes and PRs show up as the app (for example `marketer-spore[bot]`)
instead of the operator's personal token. For ad-hoc use from a
coordinator (detect, `spore gh exec`, App vs personal token), see
`spore recipes show github`.

This command is ported from the upstream team branch. Upstream also
drives a draft-PR role loop from the same config (`pr_flow =
"draft"`); that loop is not part of this build, so the keys that only
feed it are accepted but inert here (see [Config](#config)).

## One-time setup

Org-owner path: create the app once per org. Teammates joining an
existing app skip to [Teammate onboarding](#teammate-onboarding).

1. Org owner: Settings -> Developer settings -> GitHub Apps -> New.
   Permissions: Contents read/write, Pull requests read/write,
   Metadata read. No webhook. One app per org, shared by every
   teammate.
2. Install it on the target repos. Note the app id (app settings
   page) and the installation id (the number at the end of the
   installation's settings URL).
3. Generate a private key per teammate (see
   [Teammate onboarding](#teammate-onboarding)). Store it outside the
   repo, mode 0600, e.g. `~/.config/spore/<project>/github-app.pem`.
4. Point the project's `origin` at https (an ssh remote is refused:
   it would push as your own key):
   `git remote set-url origin https://github.com/<owner>/<repo>.git`.
5. Configure, then verify a token mints:

   ```
   spore gh config --set app_id=123456 --set installation_id=7890123 \
     --set private_key=~/.config/spore/<project>/github-app.pem
   spore gh token
   ```

## Teammate onboarding

Per-teammate path: the org already has its app (today
`marketer-spore`). You share that app and bring your own private key.
Below, `<pem>` is `~/.config/spore/<project>/github-app.pem`.

1. Generate a key: app settings -> "Private keys" -> "Generate a
   private key". The browser downloads a `.pem`. This is a private
   key, NOT a client secret; client secrets are for OAuth user login,
   which spore does not use.
2. On the spore host, create the secrets dir (it may not exist):
   `install -d -m 0700 ~/.config/spore/<project>`.
3. Get the `.pem` onto the host. Never paste the key into an agent
   chat or run it through the `!` prefix.
   - Paste (works over plain SSH, recommended). Copy the key on your
     laptop: `pbcopy < key.pem` (Mac), `xclip -selection clipboard <
     key.pem` or `wl-copy < key.pem` (Linux). In a fresh tmux window on
     the host run `umask 077; cat > <pem>`, paste the full PEM (BEGIN
     through END line), press Enter, then Ctrl-D, and close the window.
     Clear the clipboard after.
   - scp from the laptop:
     `scp <file>.pem <user>@<host>:~/.config/spore/<project>/github-app.pem`.
     Replace every placeholder fully: zsh reads a leftover `<` or `>`
     as a redirection. If your ssh config auto-attaches tmux
     (`RemoteCommand` / `RequestTTY`), non-interactive ssh fails with
     "open terminal failed: not a terminal"; use the paste route.
4. Verify without printing the key:
   `chmod 600 <pem>; stat -c %a <pem>; openssl pkey -in <pem> -noout && echo OK`.
   Expect `600` and `OK`.
5. Find the ids.
   - App id: the app settings page shows an "App ID" (plain integer;
     this is `app_id`) and a "Client ID" (starts with `Iv`). spore
     does not use the Client ID; ignore it.
   - Installation id: an installation is the app added to an org or
     account, with its own separate id. Org Settings -> GitHub Apps ->
     Configure on the app opens
     `github.com/organizations/<org>/settings/installations/<id>`;
     the trailing number is `installation_id`. On that page, confirm
     the app is installed on the target repos.
6. Continue with [One-time setup](#one-time-setup) steps 4-5: https
   `origin`, `spore gh config --set ...` with your own
   `private_key`, then `spore gh token`.

## Config

`$XDG_STATE_HOME/spore/<project>/github.toml` (0600), host-side, never
in the consumer repo. `<project>` is the main repo's directory name,
so every worktree of a project shares one config.

| key | meaning |
|---|---|
| `app_id`, `installation_id` | integers from step 2 |
| `private_key` | path to the PEM; a leading `~/` expands |
| `api_url` | optional REST root for the token mint; default `https://api.github.com`. Untested against GitHub Enterprise Server: `GH_ENTERPRISE_TOKEN` is set for gh, but no GHES run has verified it |
| `pr_flow` | inert in this build. Upstream, `draft` turns on the draft-PR role loop. Setting `draft` here also makes the token mint require the three keys below |
| `operator_login` | inert in this build (upstream: PR assignee for the role loop) |
| `commit_name`, `commit_email` | inert in this build (upstream: commit identity for role-loop engineer panes) |

`spore gh config` prints the file; `--set key=` deletes a key.

## Commit identity

The app is the pusher and PR author; you stay the commit author, so
GitHub lets you approve the app's PR. `spore gh exec` does not touch
commit identity: commits carry whatever `user.name` / `user.email`
the repo or the command (`git -c user.name=... commit`) sets.

## `spore gh`

- `spore gh token` prints an installation token. Tokens are cached in
  `$XDG_STATE_HOME/spore/<project>/github-token.json` (0600) until 5
  minutes before expiry; changing `app_id` or `installation_id`
  invalidates the cache.
- `spore gh exec -- <cmd...>` runs a command as the app: `GH_TOKEN`
  and `GH_ENTERPRISE_TOKEN` are set, and a git credential helper
  (username `x-access-token`, password from `$GH_TOKEN`) is injected
  through `GIT_CONFIG_COUNT` / `GIT_CONFIG_KEY_n` /
  `GIT_CONFIG_VALUE_n`, after an empty `credential.helper` that
  clears your own helpers. Both `gh` and `git push` over https then
  act as the app. It checks the `origin` of the repo in the current
  directory and refuses an ssh URL; a repo without an `origin` is not
  checked.
