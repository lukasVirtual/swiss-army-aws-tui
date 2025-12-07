# Swiss Army TUI - Agent Guidelines

This Project is using Golang 1.25. It is desinged to be a lightweight Alternative to the aws console in the web.
The Goal of this project is to mimic the aws console as much as possible to allow developers a better overview when working with AWS.

## Build Commands
- **Build executable**: Use "go build -o aws-tui"

## Code Style Guidelines
Please only write comments if they explaining edge cases etc. do not write comments to explain every piece of code.
Keep Code as simple as possible.
DO not clutter up the repo.
Use GO Best practices and follow common design guidelines when writing code.
- **Imports**: Group standard library, third-party, and internal packages. Use absolute imports for internal packages.
- **Formatting**: Use `gofmt -s` for standard formatting and simplification.
- **Naming**: PascalCase for exported, camelCase for unexported. Use descriptive names.
- **Types**: Use concrete types where possible, prefer interfaces for abstraction.
- **Error Handling**: Always handle errors explicitly, use structured logging with zap.
- **Concurrency**: Use sync.RWMutex for concurrent access, context.Context for cancellation.
- **Logging**: Use go.uber.org/zap for structured logging with appropriate levels.
- **AWS SDK**: Follow AWS SDK v2 patterns, use context with timeouts.
- **Dependencies**: Keep go.mod tidy, use specific versions.
