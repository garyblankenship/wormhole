# Serena MCP Server Installation Guide

## Quick Install Commands

### Step 1: Install Serena MCP Server
Choose the method that matches your Serena server type:

```bash
# If Serena is an npm package (most common)
npm install -g serena-mcp-server

# If Serena needs to be cloned from GitHub
git clone https://github.com/path/to/serena-mcp-server
cd serena-mcp-server
npm install
```

### Step 2: Add to Claude Code Globally

```bash
# For npm package installation
claude mcp add serena npx serena-mcp-server --scope user

# For local installation (adjust path as needed)
claude mcp add serena node /path/to/serena-mcp-server/index.js --scope user

# For binary executable
claude mcp add serena /path/to/serena-mcp-server --scope user
```

### Step 3: Verify Installation

```bash
# List all MCP servers
claude mcp list

# Should show serena in the list with ✓ Connected status
```

## Understanding MCP Scopes

| Scope | Description | Command Flag | Availability |
|-------|-------------|--------------|--------------|
| **user** | Global across all projects | `--scope user` | All your projects |
| **local** | Current project only (default) | `--scope local` | Current project |
| **project** | Shared via version control | `--scope project` | Team projects |

## Common Command Patterns

```bash
# Add with environment variables
claude mcp add serena npx serena-mcp-server --scope user -e API_KEY=your_key

# Add with custom transport (if needed)
claude mcp add serena npx serena-mcp-server --scope user --transport stdio

# Remove server if needed
claude mcp remove serena --scope user

# Get server details
claude mcp get serena
```

## Troubleshooting

### Server Not Found
```bash
# Check if the server executable exists
which serena-mcp-server

# Or check npm global packages
npm list -g | grep serena
```

### Connection Issues
```bash
# Remove and re-add the server
claude mcp remove serena --scope user
claude mcp add serena npx serena-mcp-server --scope user

# Check server health
claude mcp list
```

### Path Issues
- Use full absolute paths for local installations
- Ensure executables have proper permissions (`chmod +x`)
- Check that Node.js/npm are in your PATH

## Your Original Command (Corrected)

❌ **Your original command:**
```bash
claude mcp add serena -- <serena-mcp-server> --context ide-assistant --project $(pwd)
```

✅ **Corrected command:**
```bash
claude mcp add serena <actual-command-or-path> --scope user
```

**Key fixes:**
- Removed `--` (not needed)
- Replaced `<serena-mcp-server>` with actual command/path
- Removed `--context` (not a valid flag)
- Replaced `--project $(pwd)` with `--scope user` for global availability

## Current MCP Servers Example

Your system currently has these global servers:
- playwright, filesystem, fetch, memory, sqlite, postgres, redis, n8n-mcp

All are configured with user scope for global availability across projects.