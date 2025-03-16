# zero

Zero is a cross-platform configuration management system written in Go, inspired by tools like Chef, Puppet, and Ansible but designed to be simpler and more portable.

## Features

- Custom, easy-to-read DSL for defining system configurations
- Cross-platform support (Windows, Linux, macOS)
- Directory-based organization for platform-specific configurations
- Variables and templating
- Dependency management with intuitive syntax
- Idempotent operations
- Declarative resource model

## Installation

### From Source

1. Clone the repository
   ```
   git clone https://github.com/dangerclosesec/zero.git
   cd zero
   ```

2. Build the binary
   ```
   go build -o zero ./cmd/zero
   ```

3. Move the binary to your PATH (optional)
   ```
   sudo mv zero /usr/local/bin/
   ```

## Quick Start

1. Create a simple configuration file `example.cfg`:
   ```
   // Define a variable
   variable "web_dir" {
     value = "/var/www/html"
   }

   // Ensure web directory exists
   file "$web_dir" {
     state = "directory"
     owner = "www-data"
     group = "www-data"
     mode  = "0755"
   }

   // Create index file
   file "$web_dir/index.html" {
     content = "<html><body><h1>Hello from zero!</h1></body></html>"
     owner   = "www-data"
     group   = "www-data"
     mode    = "0644"
     
     depends_on [
       file {"$web_dir"}
     ]
   }
   ```

2. Generate a plan to see what changes would be made
   ```
   zero --plan --config example.cfg
   ```

3. Apply the configuration
   ```
   zero --apply --config example.cfg
   ```

## Configuration Organization

zero uses a directory-based structure for organizing configurations:

```
config/
├── default/               # Default configurations, shared across all platforms
│   ├── web.cfg            # Common web server configuration
│   └── base.cfg           # Base system configuration
├── linux/                 # Linux-specific configurations
│   ├── nginx.cfg          # Linux-specific nginx configuration
│   └── services.cfg       # Linux-specific service configuration
├── darwin/                # macOS-specific configurations
│   ├── homebrew.cfg       # macOS package management
│   └── launchd.cfg        # macOS service management
└── windows/               # Windows-specific configurations
    ├── iis.cfg            # Windows IIS configuration
    └── features.cfg       # Windows features configuration

main.cfg                   # Main configuration file that includes platform-specific configs
```

The main configuration file includes platform-specific files:

```
// Include all default/common configurations
include "config/default/*.cfg"

// Include platform-specific configurations
include_platform {
  linux   = "config/linux/*.cfg"
  darwin  = "config/darwin/*.cfg"
  windows = "config/windows/*.cfg"
}
```

## Configuration Syntax

zero uses a custom, easy-to-read DSL for defining configurations.

### File Resource

Manages files and directories on the system. The path is specified as the resource name.

```
file "/etc/nginx/nginx.conf" {
  content = file("templates/nginx.conf.tpl")
  owner   = "root"
  group   = "root"
  mode    = "0644"
  
  depends_on [
    package {"nginx"}
  ]
}
```

### Package Resource

Manages software packages using the system's package manager.

```
package "nginx" {
  name    = "nginx"
  state   = "installed"  // installed, removed, latest
  version = "1.18.0"     // Optional
}
```

### Service Resource

Manages system services across different init systems (systemd, upstart, launchd, Windows Services).

```
service "nginx" {
  name    = "nginx"
  state   = "running"    // running, stopped, restarted, reloaded
  enabled = true         // Start at boot
  
  depends_on [
    file {"/etc/nginx/nginx.conf"}
  ]
}
```

### Windows Feature Resource (Windows only)

Manages Windows features using DISM or PowerShell.

```
windows_feature "iis" {
  name  = "Web-Server"
  state = "installed"    // installed, removed
}
```

### Variables

Define and use variables for reusable values.

```
variable "web_root" {
  value = "/var/www/html"
}

file "$web_root/index.html" {
  content = "<html><body><h1>Hello World</h1></body></html>"
}
```

### Templates

Define reusable templates for configuration files.

```
template "default_site" {
  content = "server {\n  listen 80;\n  root $web_root;\n  index index.html;\n}"
}

file "/etc/nginx/sites-enabled/default" {
  content = template("default_site")
}
```

### Includes

Include other configuration files.

```
include "config/default/*.cfg"
```

### Platform-Specific Includes

Include files based on the current platform.

```
include_platform {
  linux   = "config/linux/*.cfg"
  darwin  = "config/darwin/*.cfg"
  windows = "config/windows/*.cfg"
}
```

### Dependencies

Specify dependencies between resources with an intuitive syntax.

```
depends_on [
  file {"/etc/nginx/nginx.conf"},
  package {"nginx"}
]
```

### Platform Conditions

Specify platform-specific resources using the `when` block:

```
when = {
  platform = ["linux", "darwin", "windows"]
}
```

## Service Management

zero provides comprehensive service management across different platforms:

### Linux

Automatically detects and uses systemd, upstart, or SysV init:

```
service "nginx" {
  name    = "nginx"
  state   = "running"
  enabled = true
}
```

### macOS

Uses launchd for service management:

```
service "nginx" {
  name    = "org.nginx.nginx"
  state   = "running"
  enabled = true
}
```

### Windows

Manages Windows services:

```
service "nginx" {
  name    = "nginx"
  state   = "running"
  enabled = true
}
```

## Command Line Options

```
zero [options]

Options:
  --config string   Path to the configuration file
  --plan            Show what changes would be made
  --apply           Apply the configuration
  --verbose         Enable verbose output
```

## Example Configuration Sets

Complete examples are available in the `examples` directory.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.