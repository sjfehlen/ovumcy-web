### Security

- **Go toolchain bumped to 1.27.4, with `go-jose` 4.1.6 and `golang.org/x/net` 0.62.0.** The module's
  `go` line and the `Dockerfile` builder stage move together, which clears GO-2026-6859 in the
  standard library's `net/http`, and the two dependency bumps clear CVE-2026-116587 (`go-jose`) and
  CVE-2026-78661 (`golang.org/x/net/html`), all reachable or flagged by the Security workflow.
