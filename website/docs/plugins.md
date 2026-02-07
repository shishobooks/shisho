---
sidebar_position: 6
---

# Plugins

Shisho supports a JavaScript plugin system that lets you extend its functionality. Plugins can hook into various stages of the book processing pipeline to customize metadata, add integrations, and more.

## Overview

Plugins are JavaScript files that run in a sandboxed environment. They can:

- Modify book metadata during library scans
- Process files after they're discovered
- Add custom metadata fields

## Installing Plugins

Plugins can be installed from a repository or loaded from local files. See the plugin documentation for your version for details on available hooks and APIs.

## Plugin SDK

The `@shisho/plugin-types` npm package provides TypeScript type definitions for building plugins. Install it in your plugin project:

```bash
npm install @shisho/plugin-types
```
