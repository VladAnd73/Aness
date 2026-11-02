# spore host migrations

Idempotent shell scripts that run on `nixos-rebuild switch` via the
bundled flake's activation hook, or by hand via `spore migrate`.

## Authoring contract

- File name: `NNN-slug.sh` where `NNN` is a zero-padded 3-digit number.
- Must be idempotent: re-running on already-migrated state must succeed
  without changing anything observable.
- Exit 0 on success, non-zero on failure. Failures halt the run; the
  ledger is not updated for the failing migration.
- Stdin is closed. Env is inherited. Working dir is `$HOME`.
- Under the activation hook, PATH holds only bash, coreutils,
  findutils, grep, sed, git, and spore. Add a tool to `migratePath` in
  `nixosModules/spore-fleet.nix` before a migration calls it.
- Bash is invoked with `bash -eu` so unset vars and command failures
  abort.
- Run as the spore user (via `runuser -u spore` in the activation
  script); migrations can read and write anything under the spore
  user's home.

## Ledger

`$XDG_STATE_HOME/spore/migrations.applied`, one line per applied
migration: `<name>\t<utc-timestamp>\t<spore-version>`. Edit by hand
only to recover from a botched migration (e.g., `sed -i` to drop a
line so a fixed migration re-runs).

## Operator commands

```
spore migrate            # apply pending
spore migrate --auto     # quiet on no-op (for activation scripts)
spore migrate --dry-run  # list pending without running
```
