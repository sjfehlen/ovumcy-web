### Changed

- **The request head limit is now 16 KB instead of 4 KB.** Cookies from other services on the same
  parent domain pushed browsers past the old read buffer and got a `431 Request Header Fields Too
  Large` before the page could load.
