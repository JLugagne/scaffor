# internal/scaffor/inbound

Driving adapters that handle external requests (CLI, MCP server).

## Overview

The `inbound` package contains adapters that receive requests from users and convert them into domain operations. It includes:

- **CLI Interface** — Command-line interface for scaffor
- **MCP Server** — Model Context Protocol server for Claude integration

## Subpackages

- `cli/commands` — CLI command implementations
- `mcp` — MCP server implementation

## Architecture

Inbound adapters are responsible for:
1. Parsing user input (command-line args, MCP requests)
2. Validating input format and constraints
3. Calling application services
4. Formatting responses for the user/client
5. Error handling and reporting

They should NOT contain business logic — that belongs in the domain layer.
