### Added

- **A per-account API key for non-browser clients.** An owner can generate a key under Settings →
  API key and hand it to another program, such as a Home Assistant integration, instead of a
  password. The key is sent as `Authorization: Bearer ovk_…`, is shown exactly once, and only a
  keyed MAC of it is stored. It carries one of two scopes, fixed when it is minted: `read`, which
  reads the cycle overview and logged days, and `period`, which adds marking a cycle start and
  setting or clearing the period flag and flow on a day. A key is accepted on exactly six
  operations and refused everywhere else, cannot manage keys, and needs no CSRF token and sets no
  cookie. Generating a new key replaces the old one, and a password reset, a recovery-code
  regeneration or a data wipe revokes it. `docs/openapi.yaml` documents the new `apiKey` security
  scheme and the two settings operations.
