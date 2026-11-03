{ config, lib, pkgs, ... }:

let
  cfg = config.services.spore-fleet;
  stateRel = ".local/state/spore/fleet-enabled";

  # effectiveProjects collapses the legacy single-project schema and
  # the multi-project schema into one attrset of { <name> = { path }; }.
  # When cfg.projectRoot is set (deprecated), it surfaces under its
  # basename so existing consumers keep working without renames.
  # Otherwise cfg.projects is used directly. The assertion below
  # guarantees at most one of the two is in effect at a time.
  effectiveProjects =
    if cfg.projectRoot != null
    then { ${baseNameOf (toString cfg.projectRoot)} = { path = cfg.projectRoot; }; }
    else cfg.projects;

  projectNames = lib.attrNames effectiveProjects;

  # Activation scripts run with a PATH that lacks bash, and the
  # migrate engine runs each migration as `bash -eu <script>`.
  migratePath = lib.makeBinPath [
    pkgs.bash
    pkgs.coreutils
    pkgs.findutils
    pkgs.gnugrep
    pkgs.gnused
    pkgs.git
    cfg.package
  ];

  # Common preamble: re-exec as cfg.user with a clean systemd-user
  # environment when invoked as root (system.activationScripts and
  # colmena pre/postActivation both run as root). When already running
  # as cfg.user (manual smoke test, or a user-context call), the
  # re-exec is skipped and the body runs in place.
  asUserPreamble = ''
    set -eu
    user='${cfg.user}'
    uid="$(${pkgs.coreutils}/bin/id -u "$user")"
    if [ "$(${pkgs.coreutils}/bin/id -un)" != "$user" ]; then
      home="$(${pkgs.getent}/bin/getent passwd "$user" | ${pkgs.coreutils}/bin/cut -d: -f6)"
      if [ -z "$home" ]; then
        echo "spore-fleet-graceful: cannot resolve home for user '$user'" >&2
        exit 1
      fi
      exec ${pkgs.util-linux}/bin/runuser -u "$user" -- ${pkgs.coreutils}/bin/env -i \
        HOME="$home" \
        USER="$user" \
        LOGNAME="$user" \
        XDG_RUNTIME_DIR="/run/user/$uid" \
        PATH="/run/current-system/sw/bin:/run/wrappers/bin" \
        "$0" "$@"
    fi
  '';

  preScript = pkgs.writeShellScriptBin "spore-fleet-graceful-pre" ''
        ${asUserPreamble}

        timeout=${toString cfg.gracefulDeploy.timeout}
        message='${cfg.gracefulDeploy.message}'
        sporecli='${cfg.package}/bin/spore'
        tmuxcli='${pkgs.tmux}/bin/tmux'

        echo "spore-fleet-graceful: disabling kill-switch" >&2
        "$sporecli" fleet disable || true

        drain_project() {
          local project="$1" project_root="$2"
          cd "$project_root"

          list_workers() {
            "$tmuxcli" list-sessions -F '#{session_name}' 2>/dev/null \
              | ${pkgs.gnugrep}/bin/grep -E "^spore/$project/" \
              | ${pkgs.gnugrep}/bin/grep -v "^spore/$project/coordinator$" || true
          }

          sessions="$(list_workers)"
          if [ -z "$sessions" ]; then
            echo "spore-fleet-graceful: [$project] no active workers" >&2
            return 0
          fi

          while IFS= read -r s; do
            slug="''${s##spore/$project/}"
            echo "spore-fleet-graceful: [$project] signalling $slug" >&2
            "$sporecli" task tell "$slug" "$message" || true
          done <<< "$sessions"

          deadline=$(( $(${pkgs.coreutils}/bin/date +%s) + timeout ))
          while [ "$(${pkgs.coreutils}/bin/date +%s)" -lt "$deadline" ]; do
            remaining="$(list_workers | ${pkgs.gnugrep}/bin/grep -c '^' || true)"
            if [ "$remaining" = "0" ]; then
              echo "spore-fleet-graceful: [$project] workers drained" >&2
              return 0
            fi
            ${pkgs.coreutils}/bin/sleep 2
          done

          echo "spore-fleet-graceful: [$project] timeout (''${timeout}s); killing remaining workers" >&2
          list_workers | while IFS= read -r s; do
            [ -z "$s" ] && continue
            "$tmuxcli" kill-session -t "$s" || true
          done
        }

    ${lib.concatMapStringsSep "\n" (name: ''
        drain_project '${name}' '${toString effectiveProjects.${name}.path}'
      '') projectNames}
  '';

  postScript = pkgs.writeShellScriptBin "spore-fleet-graceful-post" ''
    ${asUserPreamble}

    sporecli='${cfg.package}/bin/spore'
    echo "spore-fleet-graceful: re-enabling kill-switch" >&2
    "$sporecli" fleet enable
  '';

  # envSlug folds an attribute name into the SPORE_MATTER_<NAME>__<KEY>
  # shape the matter loader expects: upper-case, with non [A-Z0-9]
  # runes mapped to '_'. Mirrors internal/matter.envKey normalisation.
  envSlug = name:
    let upper = lib.toUpper name; in
    builtins.concatStringsSep "" (
      builtins.map
        (c: if (c >= "A" && c <= "Z") || (c >= "0" && c <= "9") then c else "_")
        (lib.stringToCharacters upper)
    );

  enabledMatters = lib.filterAttrs (_: m: m.enable) cfg.matters;

  # matterEnv flattens enabled matters into
  # SPORE_MATTER_<NAME>__<KEY> entries: one ENABLED=1 marker, one
  # entry per setting, and one per credentialFile (pointing at
  # $CREDENTIALS_DIRECTORY/matter-<name>-<key> via systemd's `%d`
  # specifier). The double underscore between name and key is the
  # matter loader's contract; see internal/matter/config.go envSep.
  matterEnv = lib.foldlAttrs
    (acc: name: m:
      let
        slug = envSlug name;
        prefix = "SPORE_MATTER_${slug}__";
        settingEnv = lib.mapAttrs'
          (k: v: lib.nameValuePair "${prefix}${envSlug k}" (toString v))
          m.settings;
        credEnv = lib.mapAttrs'
          (k: _: lib.nameValuePair
            "${prefix}CREDENTIAL_${envSlug k}"
            "%d/matter-${name}-${k}")
          m.credentialFiles;
      in
      acc // { "${prefix}ENABLED" = "1"; } // settingEnv // credEnv
    )
    { }
    enabledMatters;

  # matterCredentials renames per-matter credentialFiles into the flat
  # LoadCredential namespace using a matter-<name>-<key> prefix so the
  # SPORE_MATTER_<NAME>__CREDENTIAL_<KEY> env vars resolve under
  # $CREDENTIALS_DIRECTORY at runtime.
  matterCredentials = lib.foldlAttrs
    (acc: name: m:
      acc // (lib.mapAttrs'
        (k: path: lib.nameValuePair "matter-${name}-${k}" path)
        m.credentialFiles)
    )
    { }
    enabledMatters;

  matterSubmodule = lib.types.submodule {
    options = {
      enable = lib.mkEnableOption "this matter (rendered as SPORE_MATTER_<NAME>__ENABLED=1)";

      settings = lib.mkOption {
        type = lib.types.attrsOf (lib.types.oneOf [ lib.types.str lib.types.int lib.types.bool ]);
        default = { };
        example = lib.literalExpression ''{ team = "MAR"; ready_state = "Ready"; }'';
        description = ''
          Adapter-specific key-value pairs. Each entry is rendered
          to the unit env as SPORE_MATTER_<NAME>__<KEY>=<value>;
          the matter loader merges these on top of any
          [matter.<name>] section in the project's spore.toml.
        '';
      };

      credentialFiles = lib.mkOption {
        type = lib.types.attrsOf lib.types.path;
        default = { };
        example = lib.literalExpression ''{ api_key = config.age.secrets.linear-api-key.path; }'';
        description = ''
          Per-credential files exposed via systemd LoadCredential
          under the name `matter-<matter>-<key>`. The matter
          adapter receives the resolved path through
          SPORE_MATTER_<NAME>__CREDENTIAL_<KEY>=%d/matter-<matter>-<key>;
          the file itself is dereferenced by systemd at activation
          time, so secrets never enter Nix evaluation or /nix/store.
        '';
      };
    };
  };
in
{
  options.services.spore-fleet = {
    enable = lib.mkEnableOption "spore fleet reconciler (systemd-user)";

    package = lib.mkOption {
      type = lib.types.package;
      defaultText = lib.literalExpression "spore.packages.\${system}.spore";
      description = ''
        Spore CLI package. ExecStart runs `spore fleet reconcile`
        from this package on every timer tick.
      '';
    };

    claudeCodePackage = lib.mkOption {
      type = lib.types.package;
      defaultText = lib.literalExpression "spore.packages.\${system}.claude-code";
      description = ''
        claude-code CLI placed on the unit's PATH so workers spawned
        by the reconciler can invoke `claude` directly.
      '';
    };

    shimsPackage = lib.mkOption {
      type = lib.types.package;
      defaultText = lib.literalExpression "spore.packages.\${system}.shims";
      description = ''
        Spore host shims package. The module adds it to
        `environment.systemPackages` and symlinks each shim into
        `/usr/local/bin/spore-*` via a system activation script,
        so the kernel's hard-coded paths (e.g. the respawn-pane
        message from `tokenmonitor.Check` and the entries written
        to `/etc/spore/coordinator.env` by `spore infect`) keep
        resolving. The activation script sweeps any non-symlink
        target left over from the original `install -m 0755` from
        `spore infect`'s `Handoff()`.
      '';
    };

    user = lib.mkOption {
      type = lib.types.str;
      example = "spore";
      description = ''
        User account the reconciler runs under. Required: a user-
        service needs a real account to install under (the module
        does not declare the user; ensure it exists via
        `users.users.<name>` outside this module). home-manager
        wiring for this user is assumed.
      '';
    };

    projectRoot = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      example = "/home/spore/project";
      description = ''
        DEPRECATED. Single-project shorthand for the multi-project
        `services.spore-fleet.projects` schema; setting this is
        equivalent to declaring one project under
        `projects.''${baseNameOf projectRoot}.path`. Setting both
        `projectRoot` and `projects` is an error.

        Existing single-project consumers can keep this as-is for
        now; new consumers should set `projects` directly.
      '';
    };

    projects = lib.mkOption {
      type = lib.types.attrsOf (lib.types.submodule {
        options = {
          path = lib.mkOption {
            type = lib.types.path;
            example = "/home/spore/project";
            description = ''
              Project tree containing tasks/. The reconciler scans
              `''${path}/tasks` and creates worker worktrees under
              `''${path}/.worktrees/<slug>`. Must be writable by
              `services.spore-fleet.user`.
            '';
          };
        };
      });
      default = { };
      example = lib.literalExpression ''
        {
          crm-gateway.path = "/home/spore/crm-gateway";
          crm-webapp.path  = "/home/spore/crm-webapp";
        }
      '';
      description = ''
        Projects this fleet reconciles. Each entry generates its
        own `spore-fleet-reconcile-<name>` systemd-user service +
        timer + path watchers, with `WorkingDirectory` and the
        tasks/ watcher scoped to the project's path.

        The kill-switch flag at `~/.local/state/spore/fleet-enabled`
        remains host-wide; flipping it (`spore fleet enable` /
        `disable`) triggers reconciles across every project. Tmux
        session naming (`spore/<name>/coordinator`,
        `spore/<name>/<slug>`) is already namespaced by project,
        so coordinator and worker sessions stay isolated.

        Exactly one of `projects` and `projectRoot` (deprecated)
        must be set.
      '';
    };

    maxWorkers = lib.mkOption {
      type = lib.types.ints.positive;
      default = 3;
      description = ''
        Concurrency cap. Wired through SPORE_FLEET_MAX_WORKERS so
        an explicit `[fleet] max_workers` in the project's
        spore.toml still wins (matching `spore fleet reconcile`
        precedence: --max-workers > env > spore.toml > built-in
        default).
      '';
    };

    interval = lib.mkOption {
      type = lib.types.str;
      default = "60s";
      description = ''
        Timer interval between reconcile passes. Combined with the
        Path watchers on tasks/ and the kill-switch flag, so
        flipping `spore fleet enable` or committing a new active
        task is responsive even on a slow timer.
      '';
    };

    hostId = lib.mkOption {
      type = lib.types.str;
      default = config.networking.hostName;
      defaultText = lib.literalExpression "config.networking.hostName";
      description = ''
        Free-form identifier surfaced as SPORE_HOST_ID for logs and
        operator-facing chips when more than one host runs a fleet
        against the same project tree. Disambiguation only; spore
        does not coordinate across hosts.
      '';
    };

    extraEnv = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = { };
      example = lib.literalExpression ''{ SPORE_LOG = "debug"; }'';
      description = ''
        Extra entries merged into the unit's Environment=. Values
        flow through Nix evaluation and the /nix/store; never put a
        secret here. Use `credentialFiles` for those.
      '';
    };

    credentialFiles = lib.mkOption {
      type = lib.types.attrsOf lib.types.path;
      default = { };
      example = lib.literalExpression ''
        {
          github-pat = config.age.secrets.spore-github-pat.path;
        }
      '';
      description = ''
        Per-credential files exposed to the unit via systemd
        LoadCredential=. The reconciler (and the workers it
        spawns under the same unit) read decrypted material from
        the directory pointed at by $CREDENTIALS_DIRECTORY. Values
        never appear in Nix evaluation or in /nix/store; the path
        is dereferenced by systemd at activation time, so an
        agenix-decrypted file at /run/agenix/<name> works as input.

        The reconciler does NOT take an Anthropic API key from
        here. Workers spawn `claude` (claude-code), which manages
        its own credential lifecycle inside the client; this slot
        is for non-claude secrets the workers happen to need (MCP
        server keys, git-push PATs, etc.).
      '';
    };

    gracefulDeploy = {
      enable = lib.mkOption {
        type = lib.types.bool;
        default = true;
        description = ''
          Wire pre/post-activation hooks that drain active workers
          before a `nixos-rebuild switch` (or colmena deploy) and
          re-enable the reconciler after. Disable when the host is
          a one-off worker tier whose tasks should never see a
          wrap-up signal.
        '';
      };

      timeout = lib.mkOption {
        type = lib.types.ints.positive;
        default = 60;
        description = ''
          Seconds to wait for active workers to flush after the
          wrap-up signal before the pre-activation script kills
          remaining tmux sessions with SIGTERM.
        '';
      };

      message = lib.mkOption {
        type = lib.types.str;
        default = "wrap-up: deployment incoming";
        description = ''
          Body of the inbox message dropped into each active
          worker's inbox during the pre-activation drain. Workers
          should treat it as a request to flush in-progress notes
          to the task file before they get torn down.
        '';
      };

      preScript = lib.mkOption {
        type = lib.types.str;
        readOnly = true;
        description = ''
          Absolute path to the pre-activation script. Drop into
          colmena's `deployment.preActivation` to drive the same
          drain remotely. The script re-execs as
          `services.spore-fleet.user` via `runuser` and is safe to
          call from a root shell.
        '';
      };

      postScript = lib.mkOption {
        type = lib.types.str;
        readOnly = true;
        description = ''
          Absolute path to the post-activation script. Drop into
          colmena's `deployment.postActivation` to re-enable the
          fleet kill-switch after a successful deploy.
        '';
      };
    };

    matters = lib.mkOption {
      type = lib.types.attrsOf matterSubmodule;
      default = { };
      example = lib.literalExpression ''
        {
          linear = {
            enable = true;
            settings = {
              team = "MAR";
              ready_state = "Ready";
              done_state = "Done";
            };
            credentialFiles = {
              api_key = config.age.secrets.linear-api-key.path;
            };
          };
        }
      '';
      description = ''
        External work-source adapters. Each `matters.<name>` is
        rendered into the unit's environment as
        `SPORE_MATTER_<NAME>__ENABLED=1` plus one
        `SPORE_MATTER_<NAME>__<KEY>=<value>` per setting. Any
        per-matter `credentialFiles` are exposed via
        LoadCredential under the prefixed name
        `matter-<matter>-<key>`, and the resolved paths are
        passed to the adapter as
        `SPORE_MATTER_<NAME>__CREDENTIAL_<KEY>=%d/matter-<matter>-<key>`.

        The matter loader merges these env entries on top of any
        `[matter.<name>]` section in the project's spore.toml,
        so the same adapter can be configured locally via TOML
        and on a NixOS deployment via this option set.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = !(cfg.projectRoot != null && cfg.projects != { });
        message = ''
          services.spore-fleet: set either `projectRoot` (deprecated)
          or `projects`, not both. Migrate `projectRoot = X` to
          `projects.<name>.path = X`.
        '';
      }
      {
        assertion = effectiveProjects != { };
        message = ''
          services.spore-fleet.enable is true but no projects are
          configured. Set `services.spore-fleet.projects.<name>.path`
          for each project this fleet should reconcile.
        '';
      }
    ];

    warnings = lib.optional (cfg.projectRoot != null) ''
      services.spore-fleet.projectRoot is deprecated. Migrate to
      services.spore-fleet.projects.<name>.path. The current value
      is being surfaced as
      services.spore-fleet.projects."${baseNameOf (toString cfg.projectRoot)}".path
      so existing deployments keep working.
    '';

    services.spore-fleet.gracefulDeploy = {
      preScript = "${preScript}/bin/spore-fleet-graceful-pre";
      postScript = "${postScript}/bin/spore-fleet-graceful-post";
    };

    # jq + python3 are coordinator/worker workhorses (JSON parsing of
    # gh, MCP, and settings.json output; ad-hoc one-liners that outgrow
    # bash). Bundling them with the fleet module means every
    # spore-managed host has them on PATH without each host pinning its
    # own systemPackages list.
    environment.systemPackages = [
      cfg.shimsPackage
      pkgs.jq
      pkgs.python3
    ];

    # Wire the pre/post hooks into NixOS system activation so a
    # `nixos-rebuild switch` (and colmena, which lifts the same
    # activation flow on the remote) drains workers before the new
    # systemd-user units load and re-enables the kill-switch after.
    # The NIXOS_ACTION gate keeps boot-time activation a no-op: there
    # is nothing to drain on a fresh boot, and disabling the flag
    # there would leave the reconciler paused until the next deploy.
    #
    # spore-shims runs on every activation: it symlinks the six host
    # shims into /usr/local/bin/ and sweeps any non-symlink left over
    # from the original `install -m 0755` from `spore infect`.
    #
    # spore-migrate runs `spore migrate --auto` as cfg.user on every
    # activation so pending host-state migrations under
    # bootstrap/migrations/ converge on `nixos-rebuild switch`. The
    # engine is idempotent and ledger-gated; failures are suppressed
    # so a single broken migration cannot brick the rebuild path
    # (visible in `journalctl -u nixos-activation`). See
    # docs/migrations.md for the authoring contract.
    system.activationScripts = lib.mkMerge [
      {
        spore-shims = ''
          install -d -m 0755 /usr/local/bin
          for f in spore-attach spore-coordinator-launch spore-worker-brief \
                   spore-fleet-tick spore-greet-coordinator spore-greet-worker; do
            target="${cfg.shimsPackage}/bin/$f"
            link="/usr/local/bin/$f"
            if [ ! -L "$link" ] || [ "$(readlink "$link")" != "$target" ]; then
              rm -f "$link"
              ln -s "$target" "$link"
            fi
          done
        '';
        spore-migrate = ''
          ${pkgs.util-linux}/bin/runuser -u ${cfg.user} -- \
            ${pkgs.coreutils}/bin/env PATH=${migratePath} \
            ${cfg.package}/bin/spore migrate --auto || \
            echo "spore migrate: failed (see journal); continuing rebuild" >&2
        '';
      }
      (lib.mkIf cfg.gracefulDeploy.enable {
        spore-fleet-pre.text = ''
          case "''${NIXOS_ACTION:-}" in
            switch|test) ${preScript}/bin/spore-fleet-graceful-pre || true ;;
          esac
        '';
        spore-fleet-post = {
          deps = [ "spore-fleet-pre" ];
          text = ''
            case "''${NIXOS_ACTION:-}" in
              switch|test) ${postScript}/bin/spore-fleet-graceful-post || true ;;
            esac
          '';
        };
      })
    ];

    home-manager.users.${cfg.user} = {
      systemd.user.services = lib.mapAttrs'
        (name: project: lib.nameValuePair "spore-fleet-reconcile-${name}" {
          Unit = {
            Description = "spore fleet reconciler [${name}] (host=${cfg.hostId})";
          };
          Service = {
            Type = "oneshot";
            WorkingDirectory = toString project.path;
            ExecStart = "${cfg.package}/bin/spore fleet reconcile";
            Environment = lib.mapAttrsToList (n: v: "${n}=${v}") (
              {
                SPORE_FLEET_MAX_WORKERS = toString cfg.maxWorkers;
                SPORE_HOST_ID = cfg.hostId;
                # bashInteractive + coreutils land on PATH so the shims
                # the reconciler spawns (spore-coordinator-launch,
                # spore-worker-brief) can resolve `#!/usr/bin/env bash`
                # and call `cat`/`mkdir`/`tee`/`date` without an
                # in-shim PATH-augment workaround.
                PATH = lib.makeBinPath [
                  cfg.package
                  cfg.claudeCodePackage
                  pkgs.bashInteractive
                  pkgs.coreutils
                  pkgs.git
                  pkgs.tmux
                ];
              } // matterEnv // cfg.extraEnv
            );
            NoNewPrivileges = true;
            LockPersonality = true;
            RestrictSUIDSGID = true;
            ReadWritePaths = [ (toString project.path) ];
            LoadCredential = lib.mapAttrsToList
              (n: path: "${n}:${toString path}")
              (cfg.credentialFiles // matterCredentials);
          };
        })
        effectiveProjects;

      systemd.user.timers = lib.mapAttrs'
        (name: _: lib.nameValuePair "spore-fleet-reconcile-${name}" {
          Unit.Description = "Periodic spore fleet reconcile [${name}]";
          Timer = {
            OnBootSec = "30s";
            OnUnitInactiveSec = cfg.interval;
            AccuracySec = "5s";
            Unit = "spore-fleet-reconcile-${name}.service";
          };
          Install.WantedBy = [ "timers.target" ];
        })
        effectiveProjects;

      systemd.user.paths = lib.foldlAttrs
        (acc: name: project: acc // {
          "spore-fleet-reconcile-flag-${name}" = {
            Unit.Description = "Trigger spore-fleet-reconcile [${name}] when the kill-switch flag changes";
            Path = {
              PathChanged = "%h/${stateRel}";
              Unit = "spore-fleet-reconcile-${name}.service";
            };
            Install.WantedBy = [ "default.target" ];
          };

          "spore-fleet-reconcile-tasks-${name}" = {
            Unit.Description = "Trigger spore-fleet-reconcile [${name}] when tasks/ changes";
            Path = {
              PathChanged = "${toString project.path}/tasks";
              Unit = "spore-fleet-reconcile-${name}.service";
            };
            Install.WantedBy = [ "default.target" ];
          };
        })
        { }
        effectiveProjects;
    };
  };
}
