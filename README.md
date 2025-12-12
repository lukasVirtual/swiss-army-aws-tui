# Swiss Army TUI

Swiss Army TUI is a terminal UI for DevOps workflows on AWS. It provides a tabbed interface for switching between profiles/regions, inspecting resources, and viewing logs—without leaving the terminal.

## Features

### Core
- Tabbed navigation across features
- AWS profile and region switching
- Resource views with auto-refresh
- Built-in log viewer with filtering
- Configuration via file and flags

### AWS service coverage
- **EC2**: instance listing, status, and basic details
- **S3**: bucket listing and basic inspection
- **RDS**: planned
- **Lambda**: planned
- **ECS**: planned
- **VPC**: planned

### UX
- Theme support (dark/light)
- Mouse support (optional)
- Keyboard shortcuts for common actions
- Configurable refresh interval
- Filtering in list views

## Requirements
- Go 1.21+
- AWS CLI configured (recommended)
- A terminal with true color support (recommended)

## Installation

### Build from source
```bash
git clone https://github.com/yourusername/swiss-army-tui.git
cd swiss-army-tui
go build -o swiss-army-tui .
sudo mv swiss-army-tui /usr/local/bin/
```

### Install with Go
```bash
go install github.com/yourusername/swiss-army-tui@latest
```

## Quick start

Run the application (it will create a default config on first launch):
```bash
swiss-army-tui
```

Configure an AWS profile if needed:
```bash
aws configure --profile myprofile
```

Launch with an explicit profile/region:
```bash
swiss-army-tui --aws-profile myprofile --aws-region us-east-1
```

## Configuration

Config file location:
- `~/.swiss-army-tui/config.yaml`

Example:
```yaml
app:
  name: "Swiss Army TUI"
  version: "1.0.0"
  description: "DevOps AWS TUI"
  debug: false

aws:
  default_profile: "default"
  default_region: "us-east-1"
  profiles: {}

ui:
  theme: "dark"
  refresh_interval: 30
  mouse_enabled: true
  border_style: "rounded"

logger:
  level: "info"
  development: true
  encoding: "console"
  output_paths:
    - "stderr"
```

## Usage

### Navigation
- `Tab` / `Shift+Tab`: switch tabs
- `1..4`: jump to a tab
- `Ctrl+R`: refresh current view
- `Ctrl+C`: quit
- `F1` / `?`: help

### Profile tab
- `Enter`: select profile
- `Space`: test connection
- `r`: reload profiles

### Resources tab
- `Enter`: view details
- `r`: refresh
- `f`: focus filter

### Logs tab
- `r`: refresh
- `c`: clear
- `s`: toggle auto-scroll
- `g`: jump to start
- `G`: jump to end

## CLI options
```bash
swiss-army-tui [flags]

Flags:
  --aws-profile string    AWS profile to use
  --aws-region string     AWS region to use
  --config string         config file (default: $HOME/.swiss-army-tui/config.yaml)
  --dev                   enable development mode
  -h, --help              help
  --log-level string      log level (debug, info, warn, error) (default "info")
  -v, --verbose           verbose output
```

## Project layout
```text
swiss-army-tui/
├── cmd/                  # CLI entrypoints
├── internal/
│   ├── aws/              # AWS integrations (SDK clients, service wrappers)
│   ├── config/           # Config loading and validation
│   └── ui/               # TUI views/components
├── pkg/
│   └── logger/           # Logging utilities
└── main.go               # Main entry
```

### Notes on implementation
- UI is built with `tview`
- Configuration uses `viper`
- CLI uses `cobra`
- Logging uses `zap`
- AWS calls use AWS SDK for Go v2

## Development

Build:
```bash
go mod download
go build -v .
```

Test:
```bash
go test ./...
```

Run in dev mode:
```bash
go run . --dev --verbose
```

## Roadmap

### Near-term
- RDS integration
- Lambda management
- ECS service monitoring
- VPC views
- CloudWatch integration
- Export/share functionality (e.g., JSON/CSV)

### Longer-term
- Kubernetes integration
- Docker tooling
- CI/CD integrations
- IaC helpers (Terraform/CloudFormation)
- Multi-cloud support
- Plugin system

## FAQ

**How do I add a new AWS profile?**  
Add it via AWS CLI (`aws configure --profile ...`) and restart Swiss Army TUI.

**Why don’t I see any resources?**  
Confirm the selected profile/region and ensure the IAM permissions allow the relevant `Describe/List` APIs.

**Does it work with AWS SSO?**  
Yes—if your AWS CLI profile is configured for SSO, the application will use the same credential flow.

**How do I change refresh interval?**  
Update `ui.refresh_interval` in the config file or via the Settings tab (if enabled).

## License
MIT. See `LICENSE`.

## Acknowledgments
- `tview` (TUI framework)
- AWS SDK for Go v2
- `cobra` (CLI)
- `viper` (config)
- `zap` (logging)
