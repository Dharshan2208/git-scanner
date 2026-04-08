# git-scanner

Fast, concurrent secret and API token scanner for local and remote Git repositories, with optional full history analysis.

`git-scanner` walks a repository, scans files line-by-line, and reports potential secrets using:

- **Signature-based detection**: regex patterns loaded from `internal/scanner/sign.json` (currently 52 signatures).
- **Entropy-based detection**: flags high-entropy tokens to catch unknown/novel secrets.
- **Keyword based detection**: flags keywords to catch secrets

It supports scanning either the **working tree** or the **full Git commit history** (without checking out each commit so fast) using `go-git`.

## Why

Built this project as an attempt to understand how tools like `git-secrets` detect leaked credentials in repositories.

Instead of just using existing tools, I wanted to replicate the core idea myself. While doing this, learnt a lot about Go, especially around concurrency (goroutines, channels, worker pools), file scanning, and working with Git repositories programmatically.

This project is mainly for learning and experimentation, but it also reflects how such security tools work under the hood.

## Features

- Detects secrets via configurable regex signatures (`internal/scanner/sign.json`).
- Detects suspicious high-entropy strings (Shannon entropy).
- Detects secrets via keyword-based heuristics (e.g., "api_key", "token", "password").
- Deduplicates findings (file + line + type + match) and sorts results.
- Concurrent scanning using a worker pool sized to `runtime.NumCPU()`.

## Architecture

### High-Level Flow

1. CLI parses input flags (`cmd/scan.go`).
2. Repository is resolved:
   - local path is used as-is
   - remote URL is cloned into `./temp/git-scanner-*` (`internal/repo`)
3. A walker enumerates files (working tree) or tree entries (history) (`internal/walker`).
4. Jobs are processed concurrently by a worker pool (`internal/worker`).
5. Each file is scanned line-by-line (`internal/scanner`) using:
   - regex signatures loaded at startup (`internal/detector/loader.go`)
   - entropy heuristics (`internal/detector/entropy.go`)
6. Results are aggregated (dedupe + sort) (`internal/aggregator`).
7. Findings are printed and optionally written to Markdown/JSON (`internal/output`).
```

                +----------------------+
                |        CLI           |
                |  (cmd/scan.go)      |
                +----------+----------+
                           |
                           v
                +----------------------+
                |   Repo Resolver      |
                | (local / remote)     |
                +----------+----------+
                           |
                           v
        +--------------------------------------+
        |            Walker                    |
        |  (filesystem / git history tree)     |
        +----------------+---------------------+
                         |
                         v
        +--------------------------------------+
        |         Job Queue (Channel)          |
        +----------------+---------------------+
                         |
        +----------------+---------------------+
        |      Worker Pool (goroutines)        |
        |   (internal/worker, concurrent)      |
        +----------------+---------------------+
                         |
                         v
        +--------------------------------------+
        |            Scanner                   |
        | (line-by-line processing)            |
        +----------------+---------------------+
                         |
     +-------------------+-------------------+
     |                   |                   |
     v                   v                   v
+----------------+  +----------------+  +----------------------+
| Signature      |  | Keyword        |  | Entropy              |
| Detection      |  | Detection      |  | Detection            |
| (regex)        |  | (suspicious    |  | (high randomness)    |
|                |  | terms like     |  |                      |
|                |  | "api_key")     |  |                      |
+--------+-------+  +--------+-------+  +----------+-----------+
         \                 |                      /
          \                |                     /
           \               |                    /
            \              |                   /
             v             v                  v
        +--------------------------------------+
        |         Aggregator                   |
        | (dedupe + sort findings)             |
        +----------------+---------------------+
                         |
                         v
        +--------------------------------------+
        |            Output                    |
        |   (CLI / Markdown / JSON report)     |
        +--------------------------------------+
```

### Folder Structure

```text
.
├── cmd/                       # Cobra CLI commands
│   ├── root.go                # Root command ("git-scanner")
│   └── scan.go                # "scan" command + flags
├── internal/
│   ├── aggregator/            # Dedupe + sorting of findings
│   ├── detector/              # Signature loading + entropy detection
│   ├── git/                   # Git history iteration (commit trees)
│   ├── output/                # Markdown + JSON report writers
│   ├── repo/                  # Local/remote repo resolution + temp cloning
│   ├── scanner/               # Line-by-line scanning (signatures + entropy)
│   ├── types/                 # Shared data types (Finding)
│   ├── walker/                # Filesystem walker + tree walker (history)
│   └── worker/                # Worker pool and job execution
├── main.go                    # Loads signatures + executes CLI
├── go.mod / go.sum            # Go module metadata
└── temp/                      # Created at runtime for remote clones (gitignored)
```

## Installation & Setup

### Prerequisites

- Go (see `go.mod` for the declared version)

### Build

```bash
go build -o git-scanner .
```

### Run (recommended during development)

Because signatures are loaded from a **relative path** (`internal/scanner/sign.json`), run from the repository root:

```bash
go run . scan --local .
```

## Usage

### Scan a local repository

```bash
./git-scanner scan --local /path/to/repo
```

### Scan a remote repository

```bash
./git-scanner scan --repo https://github.com/OWNER/REPO
```

### Write a Markdown report (default)

```bash
./git-scanner scan --local . --output report.md
```

### Write a JSON report

```bash
./git-scanner scan --local . --output report.json --format json
```

### Scan full Git history

```bash
./git-scanner scan --local . --history --output history-report.md
```

### CLI reference

```text
git-scanner scan --local <path> | --repo <url>
  --output <file>          Path to save report (optional)
  --format markdown|json   Report format (default: markdown)
  --history                Scan commit history 
```

## Contributing

Contributions are welcome. Suggested workflow:

1. Fork the repo and create a branch(feature/fix or anything)
2. Make focused changes with clear commit messages.
3. Open a pull request

### Suggested Contribution Areas

- Add new signatures to `internal/scanner/sign.json` (include realistic ones).
- Improve false-positive filtering and entropy heuristics.
- Add configuration flags (extensions, excludes, signature path)

## Known Issues / Limitations

- **File coverage is extension-based:** only files matching `validExt` are scanned; other text files may be missed.
- **Report content may contain secrets:** Markdown/JSON reports include the matched string (often the full line). Handle reports as sensitive artifacts.
- **Duplication:** Duplicates may still appear

## License
This project is licensed under the AGPL-3.0 License — see the `LICENSE` file for details.
