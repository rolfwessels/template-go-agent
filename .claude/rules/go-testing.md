---
alwaysApply: false
description: Go testing practices for this project
paths:
    - '**/*_test.go'
---

# Go Testing Practices

- Write tests for all new features using TDD (red → green → refactor)
- If a test fails, fix the code — do not delete or weaken the test unless the behaviour is intentionally removed
- Use `testify`: `require` for fatal assertions (stops the test), `assert` for non-fatal ones
- Prefer real in-memory implementations over mocks; only mock external I/O (network, filesystem, clock)
- `// arrange`, `// act`, `// assert` section comments are acceptable for clarity

## Table-Driven Tests

Use the `tests` / `tt` convention. Prefix input fields with `give`, output fields with `want`:

```go
func TestSplitHostPort(t *testing.T) {
    tests := []struct {
        name     string
        give     string
        wantHost string
        wantPort string
    }{
        {name: "ip and port",   give: "192.0.2.0:8000", wantHost: "192.0.2.0", wantPort: "8000"},
        {name: "empty host",    give: ":8000",           wantHost: "",           wantPort: "8000"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            host, port, err := net.SplitHostPort(tt.give)
            require.NoError(t, err)
            assert.Equal(t, tt.wantHost, host)
            assert.Equal(t, tt.wantPort, port)
        })
    }
}
```

- Use `t.Run` so failures report exactly which case failed
- Subtest names should read like bug descriptions: `"missing port"`, `"admin can read suspended account"`
- Don't force a table if each case needs fundamentally different setup — write separate test functions instead
- Don't put complex conditional logic inside the loop; split into focused tables or separate tests

## Error Handling in Tests

- Use `errors.Is` / `errors.As` to check errors — never compare error strings
- Use `require.NoError` (fatal) when the test cannot meaningfully continue on error
- Use `t.Fatal` for setup failures; inside `t.Run` use `t.Fatal` to stop only that subtest

## Helper Functions

- Always call `t.Helper()` at the top of any test helper so failure lines point to the caller
- Use `t.Cleanup(func() { ... })` for teardown — no manual defer chains

```go
func setupService(t *testing.T) *Service {
    t.Helper()
    svc := NewService(NewInMemoryRepo())
    t.Cleanup(svc.Close)
    return svc
}
```

## Sample Reuse Pattern

One base builder function; derive variations from it — never duplicate struct literals:

```go
// ✅ ONE base helper
func sampleBundle(t *testing.T) Bundle {
    t.Helper()
    return Bundle{ID: "test-bundle", Version: "1.0.0", Active: true}
}

// ✅ Variation derived from base
func inactiveBundle(t *testing.T) Bundle {
    b := sampleBundle(t)
    b.Active = false
    return b
}

// ❌ Don't duplicate all fields
func inactiveBundle(t *testing.T) Bundle {
    return Bundle{ID: "test-bundle", Version: "1.0.0", Active: false}
}
```

## Parallel Tests

Inside `t.Run`, call `t.Parallel()` when safe. Go 1.22+ handles loop variables automatically; shadowing is still good practice for clarity:

```go
for _, tt := range tests {
    tt := tt // shadow for pre-1.22 safety
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel()
        // ...
    })
}
```

## Running Tests

```bash
go test ./...                                        # all tests
go test ./internal/registry/...                      # single package
go test ./internal/registry/... -run TestBundleWalker # single test
```
