# Third-party notices

Ephemeral Link is licensed under the MIT License. This file summarizes notable third-party dependencies and assets used by the project. It is provided for operator and redistributor convenience; refer to each upstream project for the authoritative license text.

## Go dependencies

- `github.com/go-chi/chi/v5` — MIT License
- `github.com/go-ldap/ldap/v3` — MIT License
- `github.com/minio/minio-go/v7` — Apache-2.0 License
- `github.com/redis/go-redis/v9` — BSD-2-Clause License
- `golang.org/x/crypto` — BSD-3-Clause License
- `golang.org/x/sys` — BSD-3-Clause License
- `github.com/cespare/xxhash/v2` — MIT License
- `github.com/dgryski/go-rendezvous` — MIT-style permissive license

## Browser libraries

- Microsoft Authentication Library for JavaScript / `@azure/msal-browser` 2.38.3 — MIT License
  - Vendored at `web/static/vendor/msal-browser-2.38.3.min.js` for Microsoft Entra ID sign-in.

## Fonts and icons

- Font Awesome Free 6.5.2 — bundled locally under `web/static/fontawesome/`; the stylesheet and webfonts are served from the application. Font Awesome icons are licensed CC BY 4.0, fonts SIL OFL 1.1, and code MIT. The complete license text is included in `web/static/fontawesome/LICENSE.txt`.

## Project assets

- `web/static/ephemeral-link-cube-fluent.svg` is the default project-owned application logo and favicon; branding settings may replace the in-page logo with an operator-uploaded image.
