# Roadmap

This roadmap distinguishes the first stable production release from larger platform features planned for a later major version.

## v1.0.0 goals

- Email delivery beyond upload-request notifications
  - Users can optionally email newly created text/file links directly to a recipient.
  - Upload-request emails and upload notifications remain supported through the same SMTP or Microsoft Graph delivery configuration.
  - Passphrases are not emailed automatically; share them through a separate channel.
- Admin analytics
  - Admins can review aggregate operational counts from active metadata and recent audit events.
  - Analytics intentionally exclude plaintext secrets, file contents, passphrases, tokens, and generated links.

## v2.0.0 goals

- S3-compatible storage
  - Add an object storage backend for encrypted file blobs while keeping local storage available.
- REST API keys
  - Add scoped, revocable API keys for programmatic creation and management of links.
- User workspaces
  - Add workspace-scoped users, links, branding, settings, audit views, and permissions.
- Custom domains
  - Add verified workspace/domain mappings so generated links can use organization-owned hostnames.
