# Third-party notices

Ephemeral Link is licensed under the MIT License. This file summarizes notable third-party dependencies and assets used by the project. It is provided for operator and redistributor convenience; refer to each upstream project for the authoritative license text.

## Go dependencies

- `github.com/go-chi/chi/v5` — MIT License
- `github.com/go-ldap/ldap/v3` — MIT License
- `github.com/redis/go-redis/v9` — BSD-2-Clause License
- `golang.org/x/crypto` — BSD-3-Clause License
- `golang.org/x/sys` — BSD-3-Clause License
- `github.com/cespare/xxhash/v2` — MIT License
- `github.com/dgryski/go-rendezvous` — MIT-style permissive license

## Browser libraries

- Microsoft Authentication Library for JavaScript / `@azure/msal-browser` 2.38.3 — MIT License
  - Vendored at `web/static/vendor/msal-browser-2.38.3.min.js` for Microsoft Entra ID sign-in.

## Fonts and icons

The production UI uses system fonts and local CSS fallback glyphs. It does not load Google Fonts or Font Awesome stylesheets.

## Project assets

- `web/static/favicon.svg` is project-owned Ephemeral Link branding artwork unless replaced by an operator-uploaded logo.
