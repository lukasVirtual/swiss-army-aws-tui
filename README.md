# Swiss Army TUI

A comprehensive Terminal User Interface (TUI) application designed for DevOps engineers. Swiss Army TUI provides a beautiful, tabbed interface to manage and monitor AWS resources, view logs, and configure settings.

## Features

### 🚀 Core Capabilities
- **Multi-tab Interface**: Intuitive tabbed navigation for different functionalities
- **AWS Profile Management**: Easy switching between AWS profiles and regions
- **Resource Monitoring**: Real-time monitoring of AWS resources
- **Log Viewing**: Integrated log viewer with filtering capabilities
- **Configuration Management**: Comprehensive settings management

### 🔧 AWS Services Support
- **EC2**: Monitor instances, their states, and configurations
- **S3**: List and manage S3 buckets
- **RDS**: Database instance monitoring (coming soon)
- **Lambda**: Function management (coming soon)
- **ECS**: Container service monitoring (coming soon)
- **VPC**: Network resource viewing (coming soon)

### 🎨 User Experience
- **Beautiful UI**: Modern terminal interface with colors and themes
- **Mouse Support**: Full mouse interaction support
- **Keyboard Shortcuts**: Efficient keyboard navigation
- **Real-time Updates**: Auto-refreshing data with configurable intervals
- **Filtering**: Powerful filtering capabilities across all views

## Installation

### Prerequisites
- Go 1.21 or later
- AWS CLI configured with profiles (optional but recommended)
- Terminal with true color support (recommended)

### From Source
```bash
git clone https://github.com/yourusername/swiss-army-tui.git
cd swiss-army-tui
go build -o swiss-army-tui .
sudo mv swiss-army-tui /usr/local/bin/
```

### Using Go Install
```bash
go install github.com/yourusername/swiss-army-tui@latest
```

## Quick Start

1. **First Run**: The application will create default configuration files
```bash
swiss-army-tui
```

2. **Configure AWS**: Ensure you have AWS profiles configured
```bash
aws configure --profile myprofile
```

3. **Launch**: Start the application and select your AWS profile
```bash
swiss-army-tui --aws-profile myprofile --aws-region us-east-1
```

## Configuration

Swiss Army TUI uses a YAML configuration file located at `~/.swiss-army-tui/config.yaml`.

### Example Configuration
```yaml
app:
  name: "Swiss Army TUI"
  version: "1.0.0"
  description: "DevOps Swiss Army Knife TUI"
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
- **Tab / Shift+Tab**: Switch between tabs
- **1, 2, 3, 4**: Jump directly to specific tabs
- **Ctrl+R**: Refresh current tab
- **Ctrl+C**: Quit application
- **F1 / ?**: Show help

### Profile Tab
- **Enter**: Select AWS profile
- **Space**: Test connection
- **r**: Refresh profiles

### Resources Tab
- **Enter**: View resource details
- **r**: Refresh resources
- **f**: Focus filter input

### Logs Tab
- **r**: Refresh logs
- **c**: Clear logs
- **s**: Toggle auto-scroll
- **g**: Go to beginning
- **G**: Go to end

## Command Line Options

```bash
swiss-army-tui [flags]

Flags:
  --aws-profile string    AWS profile to use
  --aws-region string     AWS region to use
  --config string         config file (default is $HOME/.swiss-army-tui/config.yaml)
  --dev                   enable development mode
  -h, --help              help for swiss-army-tui
  --log-level string      log level (debug, info, warn, error) (default "info")
  -v, --verbose           verbose output
```

## Architecture

Swiss Army TUI follows modern Go practices with a clean architecture:

```
swiss-army-tui/
├── cmd/                 # Command line interface
├── internal/
│   ├── aws/            # AWS service integrations
│   ├── config/         # Configuration management
│   └── ui/             # User interface components
├── pkg/
│   └── logger/         # Structured logging
└── main.go             # Application entry point
```

### Key Components

- **AWS Client**: Manages AWS SDK connections and service interactions
- **Profile Manager**: Handles AWS profile loading and validation
- **UI Framework**: Built with [tview](https://github.com/rivo/tview) for rich terminal interfaces
- **Configuration**: Uses [Viper](https://github.com/spf13/viper) for flexible configuration management
- **Logging**: Structured logging with [Zap](https://github.com/uber-go/zap)

## Development

### Building from Source
```bash
git clone https://github.com/yourusername/swiss-army-tui.git
cd swiss-army-tui
go mod download
go build -v .
```

### Running Tests
```bash
go test ./...
```

### Development Mode
```bash
go run . --dev --verbose
```

### Project Structure
The project follows Go best practices:
- `internal/`: Private application code
- `pkg/`: Public library code
- `cmd/`: Command line interface
- Clean architecture with clear separation of concerns

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines
- Follow Go best practices and idioms
- Add tests for new functionality
- Update documentation as needed
- Use conventional commit messages

## Roadmap

### Near Term
- [ ] Complete RDS integration
- [ ] Add Lambda function management
- [ ] Implement ECS service monitoring
- [ ] Add VPC network visualization
- [ ] CloudWatch integration
- [ ] Export functionality

### Long Term
- [ ] Kubernetes integration
- [ ] Docker container management
- [ ] CI/CD pipeline integration
- [ ] Infrastructure as Code support
- [ ] Multi-cloud support (Azure, GCP)
- [ ] Plugin system

## FAQ

### Q: How do I add a new AWS profile?
A: Use the AWS CLI to configure profiles, then restart Swiss Army TUI. The new profiles will be automatically detected.

### Q: Why can't I see my resources?
A: Ensure your AWS profile has the necessary permissions for the services you want to monitor.

### Q: Can I use this with AWS SSO?
A: Yes, configure your AWS profiles with SSO as you normally would, and Swiss Army TUI will use those credentials.

### Q: How do I change the refresh interval?
A: Go to the Settings tab and modify the "Refresh Interval" setting, or edit the configuration file directly.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [tview](https://github.com/rivo/tview) - Amazing TUI framework
- [AWS SDK for Go v2](https://github.com/aws/aws-sdk-go-v2) - AWS service integration
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [Zap](https://github.com/uber-go/zap) - Structured logging

## Support

If you encounter any issues or have questions:
1. Check the [FAQ](#faq) section
2. Search existing [Issues](https://github.com/yourusername/swiss-army-tui/issues)
3. Create a new issue with detailed information

---

**Made with ❤️ for the DevOps community**
# swiss-army-aws-tui
# swiss-army-aws-tui
# swiss-army-aws-tui
# swiss-army-aws-tui
# swiss-army-aws-tui
