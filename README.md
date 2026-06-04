# git-scanner

`git-scanner` is a fast, concurrent CLI tool for finding leaked secrets, API keys, tokens, and credential-like strings in Git repositories. It can scan the current working tree or inspect the full commit history, making it useful for both quick local checks and deeper repository audits.

The scanner combines three detection strategies:

- **Signature matching** against embedded regular expressions for common providers and token formats.
- **Keyword heuristics** for lines that look like they contain credentials, such as `api_key`, `token`, or `password`.
- **Entropy analysis** for high-randomness strings that may be unknown or custom secrets.

## Why This Exists

This project started as a hands-on way to understand how secret scanners work internally. Instead of treating tools like `git-secrets` as a black box, `git-scanner` rebuilds the core ideas in Go: walking repositories, scanning files line by line, using worker pools, inspecting Git history, and producing useful reports.

It is still a learning-driven project, but the goal is to keep the implementation practical, readable, and close to how a real security tool is structured.

## Features

- Scan local repositories or clone and scan remote Git repositories.
- Detect secrets using 52 embedded regex signatures from `internal/detector/sign.json`.
- Catch suspicious values with keyword and Shannon entropy heuristics.
- Scan the working tree or the full Git commit history.
- Process files and commits concurrently using worker pools.
- Deduplicate and sort findings for cleaner output.
- Write reports in Markdown or JSON.
- Sanitize matched values in console, Markdown, and JSON output to reduce accidental exposure.
- Track secret lifecycle during history scans, including exposure count and whether a finding still exists in `HEAD`.

## Installation

### Prerequisites

- Go 1.25.8 or newer, matching the version declared in `go.mod`.

### Build From Source

```bash
go build -o git-scanner .
```

### Run During Development

```bash
go run . scan --local .
```

Signatures are embedded at build time, so the scanner does not need an external signature file at runtime.

## Usage

### Scan a Local Repository

```bash
./git-scanner scan --local /path/to/repo
```

### Scan a Remote Repository

```bash
./git-scanner scan --repo https://github.com/OWNER/REPO
```

Remote repositories are cloned into a temporary directory under `./temp` and cleaned up after the scan.

### Save a Markdown Report

```bash
./git-scanner scan --local . --output report.md
```

Markdown is the default report format.

### Save a JSON Report

```bash
./git-scanner scan --local . --output report.json --format json
```

### Scan Full Git History

```bash
./git-scanner scan --local . --history --output history-report.md
```

History scans inspect commit trees directly through `go-git`, so the scanner does not need to check out each commit on disk.

## CLI Reference

```text
git-scanner scan (--local <path> | --repo <url>) [flags]

Flags:
  --local <path>           Local repository or directory to scan
  --repo <url>             Remote Git repository URL to clone and scan
  --output <file>          Optional path to save a report
  --format markdown|json   Report format when --output is used (default: markdown)
  --history                Scan the full Git commit history
```

`--local` and `--repo` are mutually exclusive. One of them is required.

## How It Works

```text
+-------------------+
| CLI               |
| cmd/scan.go       |
+---------+---------+
          |
          v
+-------------------+
| Repository        |
| local or remote   |
+---------+---------+
          |
          v
+-------------------+
| Walker            |
| files or history  |
+---------+---------+
          |
          v
+-------------------+
| Worker Pool       |
| concurrent jobs   |
+---------+---------+
          |
          v
+-------------------+
| Scanner           |
| line by line      |
+---------+---------+
          |
          v
+-------------------+
| Detectors         |
| regex, keyword,   |
| entropy           |
+---------+---------+
          |
          v
+-------------------+
| Aggregator        |
| dedupe and sort   |
+---------+---------+
          |
          v
+-------------------+
| Output            |
| console, md, json |
+-------------------+
```

The working-tree scan walks files from disk and distributes them across workers. The history scan uses `go-git` to read commit trees, scans commits in parallel, and enriches findings with lifecycle information.

## Project Structure

```text
.
├── cmd/                     # Cobra CLI commands
├── internal/
│   ├── aggregator/          # Dedupe and sorting
│   ├── detector/            # Regex signatures, keyword rules, entropy checks
│   ├── git/                 # Git history traversal
│   ├── lifecycle/           # History exposure tracking
│   ├── output/              # Console, Markdown, and JSON output
│   ├── repo/                # Local path handling and remote clone setup
│   ├── scanner/             # Line-by-line file and content scanning
│   ├── types/               # Shared finding types
│   ├── utils/               # Helpers for sanitizing and formatting
│   ├── walker/              # Filesystem and Git tree walkers
│   └── worker/              # Concurrent worker pool
├── tests/                   # Benchmarks and test assets
├── main.go                  # Signature loading and CLI entry point
├── go.mod / go.sum          # Go module metadata
└── README.md
```

## Scanning Scope

The scanner intentionally limits file coverage to common source and config formats:

```text
.go, .js, .ts, .py, .env, .json, .yaml, .yml, .txt, .c
```

It skips common dependency, build, and lockfile paths such as `.git`, `node_modules`, `vendor`, `dist`, `build`, `go.sum`, and package lockfiles. In history mode, very large files above 500 KB are skipped.

## Limitations

- File inclusion and exclusions are currently hard-coded.
- Binary files and unsupported extensions are not scanned.
- Entropy and keyword rules may flag non-secret values.
- The tool does not currently support custom signature files from the CLI.
- Remote private repositories depend on the authentication supported by `go-git` and the local environment.

## Contributing

Contributions are welcome. Good areas to improve include:

- Adding accurate signatures for more providers.
- Reducing false positives in entropy and keyword detection.
- Making file inclusion, exclusions, and signature sources configurable.
- Expanding tests for history scanning and report output.
- Improving the report format for triage workflows.

Suggested workflow:

1. Fork the repository.
2. Create a focused feature or fix branch.
3. Make the change with clear commits.
4. Open a pull request with a short explanation and any relevant test output.

## License

This project is licensed under the AGPL-3.0 License. See [LICENSE](LICENSE) for details.
