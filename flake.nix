{
  description = "spore - drop-in harness template for LLM-coding agents";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    # Claude Code's version is pinned here, on purpose, separately from
    # the nixpkgs that builds everything else. It is a second nixpkgs
    # held at one explicit revision and read for exactly one package.
    # Without it the version is a side effect of whichever nixpkgs a
    # host happens to sit on, so moving Claude Code means moving the
    # kernel and every other package at the same time. Bump this rev on
    # its own, re-run the smoke check in scripts/cc-upgrade-smoke.sh,
    # and leave the whole-system nixpkgs alone. See
    # docs/claude-code-pin.md.
    nixpkgs-claude-code.url =
      "github:NixOS/nixpkgs/55bfb4d7437bd4e09ab7cba58a43bd68eb310044";
  };

  outputs =
    { self, nixpkgs, nixpkgs-claude-code, flake-utils, home-manager }:
    let
      # The pinned nixpkgs still packages go1.26.3. Pin the toolchain to
      # the current upstream patch release from its source tarball until
      # nixpkgs catches up, then drop this overlay. The floor moves with
      # the stdlib advisories govulncheck flags as reachable: 1.26.5 for
      # GO-2026-5856 (crypto/tls), and 1.26.6 for GO-2026-6218 (net/url),
      # GO-2026-6090 (crypto/tls), GO-2026-5972 (encoding/asn1) and
      # GO-2026-5026 (net/http); 1.26.9 for the net/http, net/textproto
      # and crypto/tls batch (GO-2026-6603, 6605, 6607-6613, 6617); 1.26.10 for
      # GO-2026-6809 (crypto/x509); 1.26.11 for GO-2026-6859 (net/http).
      # Keep the spore-toolchain check's `min` in step with this version.
      goToolchainOverlay = final: prev: {
        go = prev.go.overrideAttrs (_old: {
          version = "1.26.11";
          src = prev.fetchurl {
            url = "https://go.dev/dl/go1.26.11.src.tar.gz";
            hash = "sha256-1EEBsP8n7SpMda1GwXucdDWSCeLrLKfJ32y0/baOGWk=";
          };
        });
      };
      perSystem = flake-utils.lib.eachDefaultSystem (system:
        let
          pkgs = import nixpkgs {
            inherit system;
            overlays = [ goToolchainOverlay ];
          };
          # Read from the pinned nixpkgs, not from `pkgs`. nixpkgs
          # builds Claude Code from Anthropic's own release binary, so
          # the only thing this revision decides is which release.
          # The predicate is required because the package is unfree;
          # it is scoped to this one name so nothing else slips in.
          claudeCode = (import nixpkgs-claude-code {
            inherit system;
            config.allowUnfreePredicate =
              pkg: pkgs.lib.getName pkg == "claude-code";
          }).claude-code;
          version = pkgs.lib.removeSuffix "\n" (builtins.readFile ./VERSION);
          commit =
            if self ? rev then self.rev
            else if self ? dirtyRev then self.dirtyRev
            else "unknown";

          # buildGoModule binds its own versioned go, not pkgs.go, so the
          # goToolchainOverlay above only reaches the devshell unless we
          # pass the overlaid go here too. Without this the shipped binary
          # compiles with the stock 1.26.3 stdlib and keeps GO-2026-5856.
          spore = (pkgs.buildGoModule.override { go = pkgs.go; }) {
            pname = "spore";
            inherit version;
            src = ./.;
            subPackages = [ "cmd/spore" ];
            vendorHash = null;
            ldflags = [ "-X=github.com/versality/spore.buildCommit=${commit}" ];
            # Integration tests exec git and tmux directly; without
            # them on PATH the buildGoModule check phase fails in the
            # nix sandbox. See cmd/spore/coordinator_lifecycle_test.go
            # and internal/{fleet,task,lints,bootstrap,hooks}/*_test.go.
            nativeCheckInputs = [ pkgs.git pkgs.tmux ];
            meta = {
              description = "Drop-in harness template for LLM-coding agents.";
              homepage = "https://github.com/VladAnd73/Aness";
              license = pkgs.lib.licenses.asl20;
              mainProgram = "spore";
              platforms = pkgs.lib.platforms.unix;
            };
          };

          # Host shims plus per-user hook / settings / systemd assets,
          # exposed for downstream nix configs that want to manage
          # /usr/local/bin/spore-* via nix instead of the one-shot
          # scp+install from `spore infect`. See
          # docs/host-nix-snippet.md for the operator-facing config.
          shims = pkgs.runCommand "spore-shims-${version}"
            { src = ./bootstrap/handover; } ''
            mkdir -p $out/bin $out/share/spore/hooks $out/share/spore/systemd
            install -m 0755 $src/spore-attach.sh             $out/bin/spore-attach
            install -m 0755 $src/spore-coordinator-launch.sh $out/bin/spore-coordinator-launch
            install -m 0755 $src/spore-worker-brief.sh       $out/bin/spore-worker-brief
            install -m 0755 $src/spore-fleet-tick.sh         $out/bin/spore-fleet-tick
            install -m 0755 $src/greet-coordinator.sh        $out/bin/spore-greet-coordinator
            install -m 0755 $src/greet-worker.sh             $out/bin/spore-greet-worker
            install -m 0755 $src/spore-with-secrets.sh       $out/bin/spore-with-secrets
            install -m 0755 $src/hooks/block-bg-bash.pl      $out/share/spore/hooks/block-bg-bash.pl
            install -m 0755 $src/hooks/load-state-md.pl      $out/share/spore/hooks/load-state-md.pl
            install -m 0755 $src/hooks/guard-tmux.pl         $out/share/spore/hooks/guard-tmux.pl
            install -m 0644 $src/settings.json               $out/share/spore/settings.json
            install -m 0644 $src/systemd/spore-fleet-reconcile.service $out/share/spore/systemd/spore-fleet-reconcile.service
            install -m 0644 $src/systemd/spore-fleet-reconcile.timer   $out/share/spore/systemd/spore-fleet-reconcile.timer
            install -m 0644 $src/systemd/spore-budget-daily.service    $out/share/spore/systemd/spore-budget-daily.service
            install -m 0644 $src/systemd/spore-budget-daily.timer      $out/share/spore/systemd/spore-budget-daily.timer
          '';
        in
        {
          packages = {
            inherit spore shims;
            claude-code = claudeCode;
            default = spore;
          };

          apps = {
            spore = {
              type = "app";
              program = "${spore}/bin/spore";
              meta = {
                description = "spore kernel CLI";
                mainProgram = "spore";
              };
            };
            default = {
              type = "app";
              program = "${spore}/bin/spore";
              meta = {
                description = "spore kernel CLI";
                mainProgram = "spore";
              };
            };
            claude-code = {
              type = "app";
              program = "${claudeCode}/bin/claude";
              meta = {
                description = "claude-code CLI at the revision spore pins";
                mainProgram = "claude";
              };
            };
          };

          devShells.default = pkgs.mkShell {
            packages = (with pkgs; [
              go
              golangci-lint
              govulncheck
              gopls
              just
              jq
              nixpkgs-fmt
              tmux
              fzf
              ripgrep
            ]) ++ [
              claudeCode
            ];
          };

          checks = {
            go-fmt = pkgs.runCommand "spore-go-fmt"
              {
                nativeBuildInputs = [ pkgs.go ];
              } ''
              cp -r ${./.}/. ./src
              cd src
              chmod -R u+w .
              unformatted="$(find . -name '*.go' | sort | xargs gofmt -l)"
              if [ -n "$unformatted" ]; then
                echo "gofmt needed:"
                echo "$unformatted"
                exit 1
              fi
              touch $out
            '';
            go-vet = pkgs.runCommand "spore-go-vet"
              {
                nativeBuildInputs = [ pkgs.go ];
              } ''
              cp -r ${./.}/. ./src
              cd src
              chmod -R u+w .
              export HOME=$TMPDIR
              export GOCACHE=$TMPDIR/gocache
              export GOMODCACHE=$TMPDIR/gomod
              export CGO_ENABLED=0
              go vet ./...
              touch $out
            '';
            golangci-lint = pkgs.runCommand "spore-golangci-lint"
              {
                nativeBuildInputs = [ pkgs.go pkgs.golangci-lint ];
              } ''
              cp -r ${./.}/. ./src
              cd src
              chmod -R u+w .
              export HOME=$TMPDIR
              export GOCACHE=$TMPDIR/gocache
              export GOMODCACHE=$TMPDIR/gomod
              export CGO_ENABLED=0
              golangci-lint run ./...
              touch $out
            '';
            go-test = pkgs.runCommand "spore-go-test"
              {
                nativeBuildInputs = [ pkgs.go pkgs.git pkgs.just pkgs.perl ];
              } ''
              cp -r ${./.}/. ./src
              cd src
              chmod -R u+w .
              export HOME=$TMPDIR
              export GOCACHE=$TMPDIR/gocache
              export GOMODCACHE=$TMPDIR/gomod
              export CGO_ENABLED=0
              go test ./...
              touch $out
            '';
            spore-lint = pkgs.runCommand "spore-lint"
              {
                nativeBuildInputs = [ pkgs.go pkgs.git ];
              } ''
              cp -r ${./.}/. ./src
              cd src
              chmod -R u+w .
              git init -q
              git -c user.email=t@t -c user.name=t add -A
              git -c user.email=t@t -c user.name=t commit -q -m seed
              export HOME=$TMPDIR
              export GOCACHE=$TMPDIR/gocache
              export GOMODCACHE=$TMPDIR/gomod
              export CGO_ENABLED=0
              go run ./cmd/spore lint
              touch $out
            '';
            # Load-bearing artifact gate. buildGoModule binds its own go,
            # not pkgs.go, so overlaying the toolchain only reaches the
            # devshell unless buildGoModule is told to use it too. Read the
            # Go version embedded in the SHIPPED binary and fail if it is
            # older than the vuln-clear floor (the overlay's version; see
            # the goToolchainOverlay comment). A source-only or
            # devshell-only check misses this, which is how PR #8 went
            # green with a still-vulnerable artifact.
            spore-toolchain = pkgs.runCommand "spore-toolchain"
              {
                nativeBuildInputs = [ pkgs.go ];
              } ''
              min="1.26.11"
              ver="$(go version ${spore}/bin/spore \
                | grep -oE 'go[0-9]+(\.[0-9]+)+' | head -1 | sed 's/^go//')"
              echo "artifact go toolchain: $ver (require >= $min)"
              if [ -z "$ver" ]; then
                echo "could not read go toolchain from artifact"
                exit 1
              fi
              if ! printf '%s\n%s\n' "$min" "$ver" | sort -V -C; then
                echo "artifact go $ver is older than required $min"
                exit 1
              fi
              touch $out
            '';
            # The pin's reason for existing, asserted. Claude Code's
            # version must be something a reviewer chose, not whatever
            # fell out of the host's whole-system nixpkgs revision, and
            # the floor is the first release carrying the hook fixes
            # spore depends on (docs/claude-code-pin.md). Read the
            # version off the built binary, not off the derivation's
            # `version` attribute: the attribute is what the packaging
            # claims, the binary is what a worker actually runs.
            claude-code-version = pkgs.runCommand "spore-claude-code-version" { } ''
              min="2.1.296"
              export HOME=$TMPDIR
              # stdenv sets `set -e -o pipefail`, so a bare pipeline here
              # would abort the builder at the assignment and print
              # nothing at all. Both `|| true` keep the diagnostic below
              # reachable when the binary fails or changes its format.
              raw="$(${claudeCode}/bin/claude --version 2>&1 || true)"
              ver="$(printf '%s\n' "$raw" \
                | grep -oE '[0-9]+(\.[0-9]+)+' | head -1 || true)"
              if [ -z "$ver" ]; then
                echo "could not read a version from the pinned claude-code; it printed: $raw"
                exit 1
              fi
              echo "pinned claude-code: $ver (require >= $min)"
              if ! printf '%s\n%s\n' "$min" "$ver" | sort -V -C; then
                echo "pinned claude-code $ver is older than required $min"
                exit 1
              fi
              touch $out
            '';
            shims-layout = pkgs.runCommand "spore-shims-layout" { nativeBuildInputs = [ pkgs.jq ]; } ''
              fail=0
              # Every hook script settings.json runs must ship executable.
              for h in $(jq -r '.. | .command? // empty' ${shims}/share/spore/settings.json \
                  | grep -o '/\.claude/hooks/[^ ]*' | sed 's|.*/||'); do
                if [ ! -x "${shims}/share/spore/hooks/$h" ]; then
                  echo "settings.json runs hooks/$h, which the shims package does not ship"
                  fail=1
                fi
              done
              for f in \
                bin/spore-attach \
                bin/spore-coordinator-launch \
                bin/spore-worker-brief \
                bin/spore-fleet-tick \
                bin/spore-greet-coordinator \
                bin/spore-greet-worker \
                bin/spore-with-secrets \
                share/spore/hooks/block-bg-bash.pl \
                share/spore/hooks/load-state-md.pl \
                share/spore/settings.json \
                share/spore/systemd/spore-fleet-reconcile.service \
                share/spore/systemd/spore-fleet-reconcile.timer \
                share/spore/systemd/spore-budget-daily.service \
                share/spore/systemd/spore-budget-daily.timer; do
                if [ ! -e "${shims}/$f" ]; then
                  echo "shims package missing $f"
                  fail=1
                fi
              done
              for f in \
                bin/spore-attach \
                bin/spore-coordinator-launch \
                bin/spore-worker-brief \
                bin/spore-fleet-tick \
                bin/spore-greet-coordinator \
                bin/spore-greet-worker \
                bin/spore-with-secrets \
                share/spore/hooks/block-bg-bash.pl \
                share/spore/hooks/load-state-md.pl; do
                if [ ! -x "${shims}/$f" ]; then
                  echo "shims package $f not executable"
                  fail=1
                fi
              done
              [ "$fail" = 0 ] || exit 1
              touch $out
            '';
          } // pkgs.lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
            nixosModules-spore-fleet = pkgs.testers.runNixOSTest {
              name = "spore-fleet-module";
              nodes.machine = { config, lib, pkgs, ... }: {
                imports = [
                  self.nixosModules.spore-fleet
                  home-manager.nixosModules.home-manager
                ];

                users.users.spore-test = {
                  isNormalUser = true;
                  home = "/home/spore-test";
                  createHome = true;
                  linger = true;
                };

                home-manager.useGlobalPkgs = true;
                home-manager.useUserPackages = true;
                home-manager.users.spore-test.home.stateVersion = config.system.stateVersion;

                services.spore-fleet = {
                  enable = true;
                  user = "spore-test";
                  projectRoot = "/home/spore-test/project";
                  # Stub spore CLI: log every invocation to a per-user
                  # trace file the test inspects after the activation
                  # scripts run, then handle the subset of subcommands
                  # the graceful-deploy hooks invoke.
                  package = pkgs.writeShellScriptBin "spore" ''
                    trace="''${HOME:-/tmp}/spore-stub.log"
                    echo "$*" >> "$trace"
                    case "$1 $2" in
                      "fleet reconcile") echo "stub: reconcile" ; exit 0 ;;
                      "fleet enable") : > "''${HOME}/.local/state/spore/fleet-enabled" ; exit 0 ;;
                      "fleet disable") rm -f "''${HOME}/.local/state/spore/fleet-enabled" ; exit 0 ;;
                      "task tell") shift 2; echo "stub: tell $*" ; exit 0 ;;
                      "migrate --auto") bash -c ': > /tmp/spore-migrate-found-bash' ; exit $? ;;
                      *) echo "stub spore: unknown $*" >&2; exit 1 ;;
                    esac
                  '';
                  claudeCodePackage = pkgs.writeShellScriptBin "claude" "exit 0";
                  gracefulDeploy.timeout = 5;
                  matters.linear = {
                    enable = true;
                    settings = {
                      team = "MAR";
                      ready_state = "Ready";
                    };
                    credentialFiles.api_key = pkgs.writeText "fake-linear-key" "lin_api_test";
                  };
                };

                systemd.tmpfiles.rules = [
                  "d /home/spore-test/project 0750 spore-test users -"
                  "d /home/spore-test/.local 0755 spore-test users -"
                  "d /home/spore-test/.local/state 0755 spore-test users -"
                  "d /home/spore-test/.local/state/spore 0755 spore-test users -"
                ];
              };
              testScript = { nodes, ... }:
                let
                  preScript = nodes.machine.services.spore-fleet.gracefulDeploy.preScript;
                  postScript = nodes.machine.services.spore-fleet.gracefulDeploy.postScript;
                in
                ''
                  machine.wait_for_unit("multi-user.target")
                  machine.wait_for_unit("default.target", user="spore-test")

                  # spore-shims activation: every host shim resolves
                  # via /usr/local/bin/spore-* as a symlink into the
                  # shims derivation, and each is executable.
                  for shim in [
                      "spore-attach",
                      "spore-coordinator-launch",
                      "spore-worker-brief",
                      "spore-fleet-tick",
                      "spore-greet-coordinator",
                      "spore-greet-worker",
                  ]:
                      target = machine.succeed(f"readlink /usr/local/bin/{shim}").strip()
                      assert target.startswith("/nix/store/"), \
                          f"{shim} not a /nix/store symlink: {target!r}"
                      machine.succeed(f"test -x /usr/local/bin/{shim}")
                  # Talk to the lingered user instance via systemctl
                  # --machine so the call uses the right XDG_RUNTIME_DIR
                  # without an interactive login.
                  def usercmd(cmd):
                      return f"systemctl --machine=spore-test@.host --user {cmd}"
                  # Oneshot; trigger it and assert exit 0 plus that the
                  # timer + path watchers are active.
                  machine.succeed(usercmd("start spore-fleet-reconcile-project.service"))
                  machine.succeed(usercmd("is-active spore-fleet-reconcile-project.timer"))
                  machine.succeed(usercmd("is-active spore-fleet-reconcile-flag-project.path"))
                  machine.succeed(usercmd("is-active spore-fleet-reconcile-tasks-project.path"))

                  # The matters.linear option must surface as the
                  # env-var contract the matter loader reads. Inspect
                  # the rendered unit env directly so the wire format
                  # stays pinned.
                  env = machine.succeed(usercmd(
                      "show spore-fleet-reconcile-project.service -p Environment"))
                  for needle in [
                      "SPORE_MATTER_LINEAR__ENABLED=1",
                      "SPORE_MATTER_LINEAR__TEAM=MAR",
                      "SPORE_MATTER_LINEAR__READY_STATE=Ready",
                      # systemd resolves %d to $CREDENTIALS_DIRECTORY at
                      # `show` time, so just pin the suffix.
                      "/matter-linear-api_key",
                  ]:
                      assert needle in env, f"missing {needle!r} in:\n{env}"
                  # `systemctl show -p LoadCredential` returns
                  # [unprintable] for binary values; read the unit
                  # file off disk instead. home-manager renders user
                  # units under ~/.config/systemd/user/.
                  unit = machine.succeed(
                      "cat /home/spore-test/.config/systemd/user/spore-fleet-reconcile-project.service")
                  assert "LoadCredential=matter-linear-api_key:" in unit, unit

                  # Graceful-deploy: pre-script disables the kill-switch
                  # and tells every active worker to wrap up; post-script
                  # re-enables it. With no live tmux sessions the pre-
                  # script exits after the disable + "no active workers"
                  # branch and never blocks on the timeout.
                  machine.succeed("touch /home/spore-test/.local/state/spore/fleet-enabled")
                  machine.succeed("chown spore-test:users /home/spore-test/.local/state/spore/fleet-enabled")
                  machine.succeed("${preScript}")
                  machine.fail("test -e /home/spore-test/.local/state/spore/fleet-enabled")
                  machine.succeed("${postScript}")
                  machine.succeed("test -e /home/spore-test/.local/state/spore/fleet-enabled")

                  # The pre-script must have invoked `spore fleet disable`
                  # via the stub; the trace confirms re-exec as the user
                  # and cwd handling worked.
                  machine.succeed("grep -q 'fleet disable' /home/spore-test/spore-stub.log")
                  machine.succeed("grep -q 'fleet enable' /home/spore-test/spore-stub.log")

                  # spore-migrate activation: the real binary runs each
                  # migration as `bash -eu <script>`, so bash must be on
                  # the PATH the activation hands it. The stub resolves
                  # bash the same way.
                  machine.succeed("rm -f /tmp/spore-migrate-found-bash")
                  machine.succeed("/run/current-system/activate")
                  machine.succeed("test -e /tmp/spore-migrate-found-bash")
                '';
            };
          };

          formatter = pkgs.nixpkgs-fmt;
        });
    in
    perSystem // {
      nixosModules.spore-fleet = { pkgs, lib, ... }: {
        imports = [ ./nixosModules/spore-fleet.nix ];
        services.spore-fleet.package =
          lib.mkDefault self.packages.${pkgs.stdenv.hostPlatform.system}.spore;
        services.spore-fleet.shimsPackage =
          lib.mkDefault self.packages.${pkgs.stdenv.hostPlatform.system}.shims;
        services.spore-fleet.claudeCodePackage =
          lib.mkDefault self.packages.${pkgs.stdenv.hostPlatform.system}.claude-code;
      };
      nixosModules.default = self.nixosModules.spore-fleet;
    };
}
