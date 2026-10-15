# sd

`sd` is a project task dispatcher implemented as a Go CLI.

It discovers `.sd/tasks.yaml` manifests from the current directory upward, so it works in:

- monorepos with a root `.sd/workspace.yaml` that lists Project manifests;
- standalone Project repositories that carry a `.sd/tasks.yaml`;
- arbitrary repositories that opt into SD manifests.

## Install

macOS or Linux with [Homebrew](https://brew.sh):

```sh
brew install --cask self-deprecated/tap/sd
```

Windows with [Scoop](https://scoop.sh):

```powershell
scoop bucket add self-deprecated https://github.com/self-deprecated/scoop-bucket
scoop install self-deprecated/sd
```

The fully qualified names matter: Homebrew and Scoop's main bucket both carry an unrelated `sd` (a find-and-replace tool).

With Go, or from the archives on the [releases page](https://github.com/self-deprecated/sd/releases), which include shell completions:

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
- In an assignment, a backslash-escaped quote outside quotes keeps its backslash (`v=it\'s` gives `it\'s`); write `v="it's"` instead.
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

### Current project

Inside a Project's directory, or anywhere in a repository with a single Project, the project id can be left out: `sd test` runs the current Project's `test` task, and `sd explain test` explains it. Project ids and aggregates take precedence over the current Project's task names. `sd tasks` lists the current Project's tasks; `sd tasks --all` lists every Project and aggregate.

### Flags and exit status

`--yes`, `--json`, `--mode <name>`, `--log-file <path>`, and `--var name=value` may appear anywhere before `--`. Everything after the first `--` is passed to the task untouched, so use it for arguments that look like sd flags:

```sh
sd example npm -- install --var-like-flag
```

`sd` exits with a failed task's own exit status (130 when interrupted), 2 for usage errors such as an unknown task or a missing flag value, and 1 for other errors.

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

Use repeated `--var name=value` for non-interactive overrides. A `--var` for a variable the task does not declare is an error:

```sh
sd example deploy --var target=prod --var message="ship it"
sd explain example deploy --var target=prod --var message="ship it" --json
```

Values resolve in this order, later winning: option defaults, variables derived from the selected enum values (`vars:`), choice values, mode values, and finally `--var`. If two selected values derive different values for the same variable, the task fails rather than picking one. Referenced inputs without a default are required unless `required: false` is set.

Values are never parsed as shell code. In `run`, each `{{name}}` expands to a shell variable holding the value, so `--var 'message=$(rm -rf ~)'` stays literal text whether the placeholder is double-quoted, single-quoted, or unquoted. An unquoted placeholder still splits into words, which keeps suffixes such as `publish{{publishArgs}}` with `publishArgs: " --dry-run"` working. Placeholders inside a quoted here-document (`<<'EOF'`) are not expanded. `explain` shows the command with values substituted for readability. In `dir`, `env`, and `envFiles` the value is substituted as text.

## JSON for agents

```sh
sd projects --json
sd tasks --json
sd explain demo mobile build ios-dev --json
```

JSON output is versioned. Every document has `"version": 1`; fields are always present, with empty strings, lists, and maps rather than `null`. A breaking change to these shapes will bump the version.

- `projects --json`: `{"version": 1, "projects": [{"id", "name", "aliases", "workArea", "visibility", "path"}]}`.
- `tasks --json`: `{"version": 1, "tasks": [{"project", "id", "choice", "desc", "danger", "modes", "defaultMode"}]}`. A task with choices has one row without a choice and one per choice; `project` is empty for workspace aggregates; `danger` is the danger under the default mode.
- `mirror list --json`: `{"version": 1, "mirrors": [{"id", "visibility"}]}`; `mirror explain --json` and `mirror plan --json` add `"version": 1` to the mirror configuration and plan.
- `mirror check --json`: `{"version": 1, "mirror", "passed", "exportedFiles", "findings": [{"path", "reason"}]}`. Output from configured checks goes to stderr; the exit status is non-zero when the check fails.
- `explain --json`:

```json
{
  "version": 1,
  "project": "landing",
  "id": "deploy",
  "choice": "",
  "mode": "",
  "desc": "Build and deploy via FTP",
  "run": "npm run build && ./deploy.sh",
  "dir": "/repo/projects/landing",
  "env": {},
  "envFiles": [".env"],
  "requiresEnv": ["FTP_USER", "FTP_PASSWORD"],
  "danger": true,
  "depends": [],
  "scripts": [],
  "values": {}
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

`include`, `exclude`, and `preserve` patterns match slash-separated paths relative to the Project or mirror root. `*` and `?` match within one path segment, and a `**` segment matches zero or more segments. Patterns match at any depth (`node_modules/**` also matches `apps/web/node_modules/...`, and `*.env` matches env files in any directory) unless they start with `/`, which anchors them to the root. An excluded or preserved directory covers everything below it. Malformed patterns are configuration errors.

Mirrors export regular files only. A symlink or special file that would be exported is a plan error naming the path; exclude it or replace it with a regular file. Symlinks in the destination are only allowed inside preserved paths, and `sync` never writes or deletes through a symlink. Overlay `from` and `to` paths must be relative and stay inside the Project and mirror respectively, and overlays cannot write into `.git` or `.jj`.

`plan` is dry-run and truncates long text operation lists by default; use `--all-ops` or `--op-limit <n>` to control that output. JSON plans are not truncated.

`check` runs the built-in blacklist scanner and the mirror's configured checks without syncing. The blacklist scanner rejects obvious sensitive paths and credential-shaped content before `sync`, `commit`, or `publish` modifies the mirror. Every exported file is scanned in full, whatever its size. Credential-shaped values are only accepted when they are obvious placeholders (such as `change-me`, `sk_...`, or `xxxx`), publishable client keys (`pk_live_`, `pk_test_`, and RevenueCat `appl_`/`goog_`/`amzn_` keys), or, in test files (`*.test.*`, `*_test.go`), readable fixture values made only of lowercase words, such as `provider_token_for_tests`.

After reviewing a finding that is not a real secret, such as a test fixture with a realistic-looking fake token, list its path under the mirror's `allow:` (same pattern rules as `exclude`; prefer exact file paths). Allowed findings are still printed, and reported as `allowed` in `check --json`, but no longer fail the scan:

```yaml
mirrors:
  demo:
    allow:
      - packages/sdk/test/client.test.ts
```

`sync` updates the local mirror but does not commit. `commit` and `publish` sync and create a local jj commit in the mirror; after the configured checks run, they scan every file the commit would add or modify and create no commit if anything is found. No `sd mirror` command pushes upstream; review the mirror and push manually when ready.

The destination must be the root of its own jj workspace, outside the monorepo: a destination that is, contains, or is inside the workspace root or the mirrored Project is rejected.

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

Every manifest must start with `version: 1`. Unknown keys are errors, so a typo such as `dangerous: true` fails instead of being ignored; so do duplicate project ids, an alias that is another project's id or another project's alias, and a passthrough with the same id as a task.

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
- `whenChanged`: optional glob list. If present, the task is a no-op unless a changed file matches one of the patterns. Patterns are relative to the Project root (to the workspace root for aggregates); start a pattern with `/` to make it workspace-root relative. `*` and `?` match within one path segment, `**` matches across segments, and `**/` also matches no directory at all (`**/*.go` matches `main.go`). `sd` reads changed files from `SD_CHANGED_FILES` when set (even if empty), otherwise from `SD_CHANGE_BASE` via `jj diff --from <base> --name-only` or `git diff --name-only <base>...HEAD` (failing if neither can resolve the base), otherwise from the working-copy diff via `jj diff --name-only` or `git diff --name-only HEAD`.
- `depends`: other tasks to run first, written as `project:task/path`, `project:task/path choice`, or `task/path` for an aggregate task. The task's own `run`, if any, runs after them; a choice that lists `depends` without its own `run` only runs its dependencies. Dependencies inherit the effective mode, including a default mode: a dependency with modes must define that mode, and one without modes ignores it. Dependency cycles are reported as errors.
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
