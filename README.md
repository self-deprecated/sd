# sd

`sd` is a project task dispatcher implemented as a Go CLI.

It discovers `.sd/tasks.yaml` manifests from the current directory upward, so it works in:

- monorepos with a root `.sd/workspace.yaml` that lists Project manifests;
- standalone Project repositories that carry a `.sd/tasks.yaml`;
- arbitrary repositories that opt into SD manifests.

## Install

```sh
go install github.com/self-deprecated/sd/cmd/sd@latest
```

Or with Nix: see [Nix](#nix).

## How tasks run

`run` commands, shared scripts, and mirror `checks` are executed by an embedded bash-compatible interpreter ([mvdan.cc/sh](https://github.com/mvdan/sh)), so no `bash` binary is required and a manifest behaves the same on Linux, macOS, and Windows as long as the programs it calls exist there. Common bash syntax works: functions, arrays, `[[ ]]`, `(( ))`, `set -euo pipefail`, here-strings, and `$( )`.

- Tasks inherit the environment `sd` was started with. There is no login shell, so `~/.bash_profile` and similar files are not sourced; run `sd` from the environment your tasks need (for example a devenv or `nix develop` shell).
- `trap` supports `EXIT` and `ERR` only, and `$(trap)` does not list the parent shell's traps.
- Ctrl-C stops the whole task at once, including subshells, pipelines, background jobs, and a running `EXIT` trap, and the task exits with status 130. Unlike bash, `EXIT` traps do not run after Ctrl-C, so cleanup that must survive an interrupt should not rely on them.
- External programs are looked up on `PATH` (with `PATHEXT` on Windows, so shims such as `npm.cmd` work).
- On Windows, process substitution (`<( )`) is not supported, tools such as `cat`, `rm`, or `grep` must be installed and on `PATH`, and a script with a shebang cannot be executed directly: call its interpreter explicitly, for example `bash ./scripts/build.sh`.

## Common commands

```sh
sd projects
sd tasks
sd tasks demo
sd explain demo mobile build ios-dev
sd demo mobile test
sd demo mobile build ios-dev
sd all check
```

Task paths may be written with spaces or slashes:

```sh
sd demo mobile build ios-dev
sd demo mobile/build ios-dev
```

Incomplete task paths list the next valid tasks or choices:

```sh
sd demo
sd demo mobile
sd demo mobile/build
```

## Interactive picker

When attached to a TTY, running bare `sd` opens the workspace-root picker regardless of the current Project directory; an incomplete path opens the picker at that explicit path.

Useful keys/features:

- type to filter the current stage;
- task/title matches rank before description matches;
- `Enter` drills into the highlighted item or runs the highlighted leaf task;
- `Backspace` edits the filter, then navigates back when the filter is empty;
- `Esc` clears the filter, then exits when the filter is empty;
- selected tasks show a preview of the command that will run;
- typed choice values and preview-only env values are highlighted in the command preview;
- tasks with missing required runtime inputs prompt for values after the leaf task is selected.

## Runtime variables

Tasks can declare runtime inputs under `options:` and reference them in `run`, `dir`, `env`, and `envFiles` with `{{name}}`. Supported input types are `enum`, `string`/`text`, and integer-only `integer`/`number`.

Use repeated `--var name=value` for non-interactive overrides:

```sh
sd example deploy --var target=prod --var message="ship it"
sd explain example deploy --var target=prod --var message="ship it" --json
```

Defaults apply first, then choice values, mode values, and finally explicit `--var` overrides. Referenced inputs without a default are required unless `required: false` is set.

## JSON for agents

```sh
sd projects --json
sd tasks --json
sd explain demo mobile build ios-dev --json
```

Example `explain` output:

```json
{
  "project": "landing",
  "id": "deploy",
  "desc": "Build and deploy via FTP",
  "run": "npm run build && ./deploy.sh",
  "dir": "/repo/projects/landing",
  "requiresEnv": ["FTP_USER", "FTP_PASSWORD"],
  "danger": true
}
```

## Public mirrors

`sd mirror` exports configured public-code Projects from a private source-of-truth monorepo into local public mirror checkouts. It is snapshot-based: the monorepo history stays private, and the mirror gets curated local commits.

Setup writes a local ignored path config:

```sh
sd mirror init --root ~/path/to/public-mirrors
```

The committed `.sd/mirrors.yaml` stores portable mirror rules and named destination roots. Personal filesystem paths live in ignored `.sd/mirrors.local.yaml`, or can be supplied with `SD_PUBLIC_MIRRORS` / `--mirror-root`.

Common commands:

```sh
sd mirror
sd mirror list
sd mirror explain demo
sd mirror plan demo
sd mirror check demo
sd mirror sync demo
sd mirror commit demo -m "feat: public change"
sd mirror publish demo -m "feat: public change"
```

When attached to a TTY, bare `sd mirror` opens an interactive picker for the configured mirror and action. Running an action without a mirror, such as `sd mirror plan`, also prompts for the mirror. Non-interactive runs keep requiring explicit mirror ids.

`plan` is dry-run and truncates long text operation lists by default; use `--all-ops` or `--op-limit <n>` to control that output. JSON plans are not truncated.

`check` runs the built-in blacklist scanner and the mirror's configured checks without syncing. The blacklist scanner rejects obvious sensitive paths and credential-shaped content before `sync`, `commit`, or `publish` modifies the mirror.

`sync` updates the local mirror but does not commit. `commit` and `publish` sync and create a local jj commit in the mirror. No `sd mirror` command pushes upstream; review the mirror and push manually when ready.

If a required destination root is missing, `sd mirror` errors with setup instructions. If a destination mirror has uncommitted local changes, interactive runs warn and ask whether to continue; non-interactive runs require `--yes`.

## Shell completions

Installed packages include Bash, Zsh, and Fish completions. Completions can also be generated manually:

```sh
sd completion bash
sd completion zsh
sd completion fish
sd completion powershell
```

## Nix

Run directly:

```sh
nix run github:self-deprecated/sd -- --help
```

Install from a NixOS flake:

```nix
{
  inputs.sd-cli.url = "github:self-deprecated/sd";
}
```

```nix
{ inputs, pkgs, ... }:

{
  environment.systemPackages = [
    inputs.sd-cli.packages.${pkgs.system}.sd
  ];
}
```

Home Manager:

```nix
{ inputs, pkgs, ... }:

{
  home.packages = [
    inputs.sd-cli.packages.${pkgs.system}.sd
  ];
}
```

## Adding a Project task

1. Edit the Project-local manifest: `projects/<project>/.sd/tasks.yaml`.
2. Add a task under `tasks:` with a stable id, short `desc`, and `run` command.
3. Use nested ids such as `mobile/build` when a task belongs to a group.
4. Use `choices:` for variants of one task instead of creating pseudo-task paths.
5. Run `sd tasks <project>` and `sd explain <project> <task...>` to verify the surface before running it.

Example:

```yaml
tasks:
  check:
    desc: Run checks
    whenChanged:
      - projects/example/**
    run: npm test

  deploy:
    desc: Deploy the site
    danger: true
    envFiles: [.env]
    requiresEnv: [TOKEN]
    run: ./deploy.sh
```

## Manifest reference

Project manifests live at `.sd/tasks.yaml`:

```yaml
version: 1

project:
  id: example
  name: Example Project
  aliases: [ex]
  workArea: Product
  visibility: public-code
  dir: .

tasks:
  check:
    desc: Run checks
    run: npm test
```

### Project fields

- `id`: canonical Project id used on the command line.
- `name`: human-readable Project name.
- `aliases`: optional alternate ids.
- `workArea`: grouping label used by `sd projects`.
- `visibility`: inventory/role label.
- `dir`: path to the Project root, relative to the manifest directory. Defaults to `..` for Project manifests in `.sd/`.
- `scripts`: top-level map of named shared shell fragments available to this Project's tasks.

### Task fields

- `desc`: short description shown in listings and the picker.
- `run`: shell command run from the task working directory.
- `dir`: optional task working directory, relative to the Project root.
- `env`: environment variables added for the task. These are also shown in previews.
- `envFiles`: optional `.env`-style files loaded before the task runs. Relative paths are resolved from the task working directory; missing files are ignored so shell env can still satisfy `requiresEnv`. Later files override earlier files; shell env overrides file values; task `env` overrides both.
- `requiresEnv`: environment variables that must be non-empty after `envFiles`, the shell environment, and task `env` are merged.
- `danger`: require `--yes` / confirmation before running.
- `whenChanged`: optional workspace-root glob list. If present, the task is a no-op unless changed files match one of the patterns. `sd` reads changed files from `SD_CHANGED_FILES` when set (even if empty), then `SD_CHANGE_BASE` via `jj diff --from <base> --name-only` / Git range diff, then the current working-copy diff via `jj diff --name-only` / `git diff --name-only HEAD`.
- `depends`: other tasks to run first, written as `project:task/path`, `project:task/path choice`, or `task/path` for an aggregate task.
- `scripts`: names of project-level shared scripts to run in order in the same shell immediately before `run`.
- `options`: typed inputs used by choices and `{{template}}` rendering.
- `choices`: named presets for one task.
- `modes` / `defaultMode`: named run modes selected with `--mode <name>` or Tab in the picker; modes supply template `values`, env, and optional run/dir/danger overrides.

### Choices and templating

Use choices when variants share one conceptual task:

```yaml
tasks:
  mobile/build:
    desc: Build the mobile app
    run: eas build --platform {{platform}} --profile {{profile}}
    options:
      platform:
        type: enum
        prompt: Platform
        values:
          ios:
            label: iOS
          android:
            label: Android
      profile:
        type: enum
        prompt: Profile
        default: development
        values:
          development:
            label: Development
          production:
            label: Production
    choices:
      ios-dev:
        label: iOS development
        values:
          platform: ios
          profile: development
      ios-production:
        label: iOS production
        danger: true
        values:
          platform: ios
          profile: production
```

Choice fields can override or extend task fields:

- `label` / `desc`: display text for the choice.
- `values`: values used for `{{name}}` template rendering.
- `run`, `dir`, `env`, `envFiles`, `requiresEnv`, `danger`, `depends`, `scripts`, `modes`, `defaultMode`: choice-specific task behavior.

### Shared scripts

Project manifests can define named shell fragments once and attach them to tasks, choices, or modes. Shared scripts and the task command execute in one shell, so exported variables and functions remain available to the task. A failing script stops the task. Danger confirmation happens before any shared script executes.

```yaml
scripts:
  deployment-context:
    desc: Resolve the current deployment target
    run: |
      export DEPLOYMENT_TARGET="$(./scripts/current-target)"

tasks:
  deploy:
    scripts: [deployment-context]
    danger: true
    run: ./deploy "$DEPLOYMENT_TARGET"
```

Task scripts run first, followed by scripts added by a selected choice and then by the selected mode. Duplicate names are run once at their first position. Unknown names and empty shared scripts fail while loading the manifest. `sd explain` lists shared script names without executing them.

Modes can provide task-wide variables such as a dry-run suffix, and choices can override individual modes when needed:

```yaml
tasks:
  release/npm:
    danger: true
    defaultMode: publish
    modes:
      publish:
        values: {publishArgs: ""}
      dry-run:
        danger: false
        values: {publishArgs: " --dry-run"}
    choices:
      cli:
        run: npm publish{{publishArgs}}
      all:
        depends:
          - app:release/npm cli
```

Option values may also define extra template variables:

```yaml
options:
  target:
    type: enum
    values:
      apk:
        label: APK
        vars:
          profile: preview
```

Options without choices are collected at runtime. In an interactive TTY, `sd` prompts for missing required values; non-interactive runs must pass them with `--var`:

```yaml
tasks:
  deploy:
    desc: Deploy one target
    dir: deploy/{{target}}
    envFiles:
      - ../../env/{{target}}.env
    env:
      DEPLOY_COUNT: "{{count}}"
    run: ./deploy.sh --message "{{message}}" --profile {{profile}}
    options:
      target:
        type: enum
        prompt: Target
        default: dev
        values:
          dev:
            label: Development
            vars: {profile: debug}
          prod:
            label: Production
            vars: {profile: release}
      message:
        type: text
        prompt: Release message
      count:
        type: integer
        default: "1"
```

### Passthrough commands

Use `passthrough:` for explicit package-manager/tool escape hatches:

```yaml
passthrough:
  npm:
    desc: Run npm in the Project
    run: npm "$@"
```

Then run:

```sh
sd example npm -- install left-pad
```

### Workspace and aggregates

Workspace discovery lives in root `.sd/workspace.yaml`; cross-Project aggregate tasks live in root `.sd/aggregates.yaml`. Keep Project-owned tasks in Project-local `.sd/tasks.yaml` so standalone Project repositories can carry their own task surface.

Aggregate example:

```yaml
version: 1

tasks:
  all/check:
    desc: Run conservative checks
    depends:
      - demo:check
      - landing:check
```

## Troubleshooting

- Use `sd explain <project> <task...>` before running a task to inspect command, working directory, env, env files, required env, and danger status.
- If a task needs credentials, add them to your shell environment or a task `envFiles` file such as `.env`, and list the variable names in `requiresEnv`.
- If a task variant only changes env or template values, model it as a `choice` so preview and execution stay aligned.
- If Nix cannot see a newly added file in a jj or Git repository, snapshot/commit (or `git add`) the working copy before rerunning the Nix command.
