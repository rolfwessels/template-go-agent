---
name: go-development
description: Apply this project’s Go coding practices when creating, modifying, reviewing, or debugging Go source files. Covers design, errors, dependencies, and required verification; do not use for non-Go work.
---


# General Go Coding Practices

- Follow SOLID principles
- **DRY**: if you notice duplication, extract a helper function; functions differing only in a single parameter should share a common implementation
- Use the least and simplest code to solve the problem
- **ONLY implement what is explicitly specified** — do not add extra features or functionality that aren't required; ask first if you think something is missing
- Keep functions small (aim for under 15 lines); break large functions into smaller, well-named helpers
- Write testable code; use interface-based dependency injection
- **DO NOT add comments** that narrate what the code does; if a comment feels necessary, refactor into a named function instead
- `// arrange`, `// act`, `// assert` comments are acceptable inside test functions
- **ALWAYS run tests after making changes**: `docker compose exec dev make test` (runs vet + tests inside the dev container)
- Code must pass `make vet` with no warnings; `make test` runs vet first as a prerequisite
- Add new dependencies with `docker compose exec dev go get <module>`, never edit `go.mod` directly
- Prefer explicit error handling over panics; always wrap errors with context using `fmt.Errorf("doing X: %w", err)`
- Sentinel errors use `errors.New` and an `Err` prefix: `var ErrNotFound = errors.New("not found")`; check with `errors.Is` / `errors.As`, never string comparison
- Implement backward-compatible changes: use functional-options or optional parameters instead of breaking existing call sites

## Method Extraction Example

```go
// ❌ Don't
func processOrder(repo Repository, email Emailer, orderID string) error {
    order, err := repo.Find(orderID)
    if err != nil || order == nil {
        return fmt.Errorf("order not found: %w", err)
    }
    subtotal := calculateSubtotal(order.Items)
    total := subtotal + subtotal*0.2
    body := fmt.Sprintf("Your order %s total is %.2f", orderID, total)
    return email.Send(order.CustomerEmail, "Order Confirmation", body)
}

// ✅ Do
func processOrder(repo Repository, email Emailer, orderID string) error {
    order, err := validateAndGetOrder(repo, orderID)
    if err != nil { return err }
    total := calculateOrderTotal(order)
    return sendConfirmationEmail(email, order, total)
}
```
