package task

import (
	"fmt"
	"os/exec"

	"github.com/versality/spore/internal/task/frontmatter"
)

// netnsBinary is the userspace-NAT binary that provides each worker its
// own private network namespace with outbound internet + DNS. pasta
// ships in the `passt` package (bundled in the spore flake). Verified
// primitive: `pasta --config-net -- <cmd>` runs <cmd> in a fresh
// user+net namespace with a configured uplink, DNS, and a private
// loopback, so N workers reuse identical ports blind to each other
// while host ports stay free. No sudo, no nested unshare needed.
const netnsBinary = "pasta"

// netnsWrapPrefix is prepended to the resolved worker command when
// [worker] isolate_network is on. Everything after `--` is the command
// pasta execs inside the namespace. `pasta --config-net` maps the worker
// to uid 0 in a user namespace; claude refuses to run as root, so we
// inject its sandbox-bypass env via coreutils `env` (on the netns PATH).
// This is legitimate: the worker IS sandboxed in its own netns and the
// uid-0 is only a userns mapping to the unprivileged spore user, so no
// real privilege is gained. The env injection is tied to isolation and
// never applied when isolate_network is off.
//
// -T none -U none -t none -u none disable pasta's default `auto` port
// forwarding in BOTH directions: -T/-U (namespace->host, "to init") and
// -t/-u (host->namespace). Without them, any port a worker binds inside
// its netns is auto-forwarded to the host, so two isolated workers on the
// same port (e.g. :3000) collide host-side nondeterministically. `none`
// keeps host ports free. Outbound NAT (internet + DNS) rides pasta's tap
// interface and copied routes, independent of these flags, so isolation
// and internet both keep working. Flags precede `--` (pasta args).
const netnsWrapPrefix = netnsRunPrefix + sandboxEnv

const (
	netnsRunPrefix = netnsBinary + " --config-net -T none -U none -t none -u none -- "
	sandboxEnv     = "env IS_SANDBOX=1 "
)

// lookPath is exec.LookPath, indirected so tests can stub binary
// discovery without touching the real PATH.
var lookPath = exec.LookPath

// wrapForIsolation returns the command tmux should run for a worker. When
// isolate is false it returns agent unchanged. When true it verifies the
// netns binary is on PATH (failing loud with an actionable error if not)
// and prefixes the pasta netns wrapper.
func wrapForIsolation(agent string, isolate bool) (string, error) {
	if !isolate {
		return agent, nil
	}
	if _, err := lookPath(netnsBinary); err != nil {
		return "", fmt.Errorf(
			"[worker] isolate_network is on but %q is not on PATH: install passt (nix: nixpkgs#passt, or add it to your host flake) or set isolate_network = false: %w",
			netnsBinary, err,
		)
	}
	return netnsWrapPrefix + agent, nil
}

// workerSpawnCommand resolves the command tmux runs for a worker: the
// agent binary (env > frontmatter > default) optionally wrapped in the
// netns launcher when the project's [worker] isolate_network is on. It
// is the composition seam ensureSession reaches through
// workerLaunchCommand, kept pure so
// the off/on wrap and the missing-pasta fail-loud path are testable
// without spawning tmux. Coordinators never call this: they spawn via
// internal/fleet/coordinator.go, so isolation cannot leak onto them.
func workerSpawnCommand(projectRoot string, m frontmatter.Meta) (string, error) {
	agent, err := workerAgentCommand(m)
	if err != nil {
		return "", err
	}
	cfg, err := LoadWorkerConfig(projectRoot)
	if err != nil {
		return "", err
	}
	return wrapForIsolation(agent, cfg.IsolateNetwork)
}

// Isolation is a project's resolved [worker] isolate_network.
type Isolation struct{ on bool }

// LoadIsolation reads isolate_network and, when it is on, fails loud
// if pasta is missing, as wrapForIsolation does.
func LoadIsolation(projectRoot string) (Isolation, error) {
	cfg, err := LoadWorkerConfig(projectRoot)
	if err != nil {
		return Isolation{}, err
	}
	if _, err := wrapForIsolation("", cfg.IsolateNetwork); err != nil {
		return Isolation{}, err
	}
	return Isolation{on: cfg.IsolateNetwork}, nil
}

// Wrap runs agent alone inside the namespace.
func (i Isolation) Wrap(agent string) string {
	if !i.on {
		return agent
	}
	return netnsWrapPrefix + agent
}

// Agent is agent as it runs inside a namespace Script set up.
func (i Isolation) Agent(agent string) string {
	if !i.on {
		return agent
	}
	return sandboxEnv + agent
}

// Script runs a whole launch script inside the namespace, so a
// pre-start hook and the agent share one loopback: services the hook
// starts are reachable from the agent. Only the agent gets
// IS_SANDBOX, through Agent.
func (i Isolation) Script(script string) string {
	if !i.on {
		return script
	}
	return netnsRunPrefix + "sh -c " + shellQuote(script)
}

// workerLaunchCommand is the command tmux runs for a worker: the
// agent behind the [fleet] pre_start hook when one is set.
func workerLaunchCommand(projectRoot, slug string, m frontmatter.Meta) (string, error) {
	prelude := LoadPreStart(projectRoot).Prelude(projectRoot, slug, "worker")
	if prelude == "" {
		return workerSpawnCommand(projectRoot, m)
	}
	agent, err := workerAgentCommand(m)
	if err != nil {
		return "", err
	}
	iso, err := LoadIsolation(projectRoot)
	if err != nil {
		return "", err
	}
	return iso.Script(workerShellCommand(prelude, iso.Agent(agent))), nil
}
