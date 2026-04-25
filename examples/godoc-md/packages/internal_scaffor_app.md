# internal/scaffor/app

Application layer implementing use cases and command handlers.

## Overview

The `app` package contains application services that orchestrate domain logic and coordinate between different layers. It implements the use cases of the application (executing templates, linting, testing, etc.).

## Subpackages

- `commands` — Command handlers and orchestration logic

## Architecture

The app layer sits between:
- **Inbound adapters** (CLI, MCP) that receive user requests
- **Domain models** that define the core logic
- **Outbound adapters** (filesystem, repositories) that implement persistence

This ensures clean separation of concerns and testability.
