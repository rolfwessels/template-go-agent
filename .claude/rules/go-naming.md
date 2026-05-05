---
alwaysApply: false
description: Go naming conventions for this project
paths:
    - '**/*.go'
---

# Go Naming Conventions

## Packages
- Lowercase, single word, no underscores: `registry`, `walker`, `config`
- Not plural: `net/url` not `net/urls`
- Never `util`, `common`, `shared`, `lib`, or `helpers` — name what it provides
- Do not repeat the package name in function names:
  ```go
  // ❌ package yamlconfig → func ParseYAMLConfig(...)
  // ✅ package yamlconfig → func Parse(...)
  ```

## Functions & Methods
- `MixedCaps` for exported, `camelCase` for unexported — no underscores except in test names
- Functions that **return something** get noun-like names; functions that **do something** get verb-like names
- Omit `Get` prefix on getter methods: `Name()` not `GetName()`
- Do not repeat the receiver type in the method name:
  ```go
  // ❌ func (c *Config) WriteConfigTo(w io.Writer)
  // ✅ func (c *Config) WriteTo(w io.Writer)
  ```
- Acronyms stay uppercase: `parseURL`, `HTTPClient`, `writeJSON`
- When type disambiguation is needed, append the type: `ParseInt`, `ParseInt64`

## Test Functions
- Top-level: `TestFunctionName_WhatIsBeingTested` (underscores allowed here only)
- Subtest names in `t.Run`: plain prose describing the scenario — `"missing port"`, `"empty header is rejected"`

## Variables & Parameters
- Short names for small scopes (`i`, `k`, `v` in loops)
- Avoid stuttering: `registry.Entry` not `registry.RegistryEntry`
- Sentinel errors: `ErrNotFound`, `ErrInvalidConfig` (exported `Err` prefix)

## Interfaces
- Single-method: method name + `-er` suffix — `Walker`, `Reader`, `Syncer`
- Multi-method: describe the role — `BundleStore`, `ConfigWriter`
- Define interfaces where they are **consumed**, not where they are implemented

## Files
- Lowercase with underscores: `bundle_walker.go`, `config_writer.go`
- Test files: `bundle_walker_test.go`
- One primary type per file where practical
