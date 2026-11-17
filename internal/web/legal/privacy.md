---
title: Privacy Policy
service: Ephemeral Link
---

> **Before you publish this:** This is a template, not legal advice. Ephemeral Link is
> self-hosted — every operator runs their own instance and is the **data controller**
> for that instance under the GDPR. Replace every `[bracketed]` placeholder with your
> own details before putting this on a live site. If you're hosting for customers or
> at any commercial scale in Germany, get this checked by a lawyer (a *Datenschutz*-
> or IT-law specialist), and note that a German-hosted site normally also needs a
> separate **Impressum** (legal notice, §5 TMG/DDG) — this document does not replace one.

# Privacy Policy

Effective date: **[Date]**

This Privacy Policy explains how **[Operator name / company]** ("**we**," "**us**")
process personal data when you use Ephemeral Link at **[your-domain.tld]** (the
"**Service**"). We built the Service to minimize the data we hold about you. This policy describes the
categories of data processed by a standard deployment; the operator must adapt it to
actual hosting, logging, integrations, and retention settings before publication.

## Summary

| Section | What's there |
|---|---|
| [A. Who we are](#a-who-we-are) | The controller responsible for this instance. |
| [B. What the Service does](#b-what-the-service-does) | How Ephemeral Link handles the secrets you create. |
| [C. What data we process](#c-what-data-we-process) | The specific data points, and why. |
| [D. Legal basis](#d-legal-basis) | Why processing this data is lawful under the GDPR. |
| [E. Retention and deletion](#e-retention-and-deletion) | How long anything sticks around. |
| [F. Cookies and local storage](#f-cookies-and-local-storage) | What, if anything, we store in your browser. |
| [G. Recipients and hosting](#g-recipients-and-hosting) | Where the server lives and who else sees data. |
| [H. International transfers](#h-international-transfers) | Whether data leaves the EU/EEA. |
| [I. Your rights](#i-your-rights) | Access, deletion, complaints, and how to use them. |
| [J. Security](#j-security) | How we protect what little we hold. |
| [K. Children](#k-children) | Age restrictions. |
| [L. Changes to this policy](#l-changes-to-this-policy) | How you'll hear about updates. |
| [M. Contact](#m-contact) | How to reach us. |

## A. Who We Are

**Controller:** [Operator name / company]
**Address:** [Street, postal code, city, country]
**Email:** [privacy@your-domain.tld]

[If applicable: **Data Protection Officer:** [Name / contact], reachable at [email].]

## B. What the Service Does

Ephemeral Link lets a sender create a short text secret or encrypted file behind a
one-time link. A valid recipient request atomically claims the item; the protected
payload is then removed from the application record and the item cannot be claimed
again. A failed transfer does not restore a claimed item. Expired items are removed by
scheduled cleanup. We do not use secret contents for advertising, profiling, or
analytics, but the operator may technically access data needed to operate, secure, or
restore the deployment.

## C. What Data We Process

We keep this as small as possible:

- **Secret content and files.** Text and files submitted for sharing. In the standard
  deployment, payloads are encrypted at rest with AES-256-GCM and are retained until
  successful one-time claim or expiry. File names, MIME types, and file sizes may be
  stored as item metadata.
- **Links and item metadata.** Random link identifiers, creation/expiry times, status,
  workspace, and whether a passphrase is configured. The full generated link may be
  present temporarily in the browser session history described below.
- **Upload-request and email data.** If enabled, requester/recipient email addresses,
  messages, delivery status, and related expiry/status metadata. Configured SMTP or
  Microsoft Graph providers may receive the recipient address, message, and generated
  link needed to deliver an email.
- **Accounts and access data.** Usernames, names, email addresses, roles, workspace
  memberships, password hashes, external identity-provider identifiers, session
  records, API-key metadata, and invitation metadata when those features are enabled.
  The Service does not store local passwords in plaintext. External providers may
  process authentication data under their own policies.
- **Security and audit data.** Audit events may include actor, event, target, result,
  timestamp, source IP address, and short operational details. The standard deployment
  retains audit records for 90 days, subject to the operator's configuration and
  backups. Reverse proxies, hosting providers, SMTP/Graph providers, object storage,
  and other infrastructure may also create their own logs.

The standard application does not run advertising or analytics scripts. The operator
must update this statement if additional services are installed.

## D. Legal Basis

Under Art. 6(1) GDPR, the operator should document the applicable basis for the
specific deployment. Common bases include:

- **Art. 6(1)(b)** — providing the requested sharing, account, workspace, and email
  delivery features.
- **Art. 6(1)(f)** — securing the Service, preventing abuse, maintaining audit trails,
  and protecting the operator's infrastructure, balanced against the individual's
  rights.
- **Art. 6(1)(c)** — complying with legal obligations, where a retention or disclosure
  is required by law.

Do not publish this section without confirming the operator's actual legal bases and,
where required, obtaining consent for optional processing.

## E. Retention and Deletion

- After a valid one-time claim, the encrypted payload and key material are removed from
  the active item record. Minimal consumed-item metadata can remain temporarily for
  operational cleanup; in the standard deployment the Redis record expires after one
  hour. Expired items are removed according to the configured TTL and cleanup process.
- Audit records are retained for 90 days in the standard deployment. Confirm the
  retention of reverse-proxy, host, Redis, object-storage, and email-provider logs:
  **[insert actual periods]**.
- Browser session history is cleared when the browser session ends, subject to browser
  behavior. A theme preference may remain in browser local storage until the user or
  browser removes it.
- **[Describe encrypted backup retention, or state that no backups of active payloads
  are kept. State how deletion requests apply to backups.]**

## F. Cookies and Local Storage

The Service uses strictly necessary cookies for authentication, CSRF protection,
language preference, and the short-lived created-link receipt flow. The session cookie
is HttpOnly and uses SameSite protections; the Secure flag depends on the deployment's
HTTPS configuration. The application also stores the selected light/dark theme in
browser local storage and may store up to 25 generated-link receipts, including full
links, in browser session storage to show local history. These browser stores are not
sent to the Service as analytics, but anyone with access to the browser profile may be
able to view them. The operator must update this section if consent-managed or third-
party cookies are added.

## G. Recipients and Hosting

The Service runs on infrastructure operated by **[hosting provider name]**, with
servers located in **[data center location]**. **[State whether a Data Processing
Agreement under Art. 28 GDPR is in place.]**

The operator may use Redis/Valkey, local encrypted storage or S3-compatible object
storage, SMTP, Microsoft Graph, directory services, reverse proxies, and hosting
providers. List each provider, purpose, location, and applicable safeguards here before
publication. We do not sell personal data or secret content. Providers may process data
only as needed to provide their configured service and under their own terms and
privacy policies.

## H. International Transfers

**[State whether processing stays within the EU/EEA. If data is transferred outside it,
identify each recipient country and the applicable safeguard, such as an adequacy
decision or Standard Contractual Clauses.]**

## I. Your Rights

If you're in the EU/EEA, you may request access, rectification, erasure, restriction,
data portability, or object to processing based on legitimate interests. You may also
lodge a complaint with the supervisory authority in your country or German state.

Because the Service holds short-lived data and secrets self-destruct, some requests
may already be moot when received. The operator may need reasonable information to
verify a request and protect another person's secret or account. Send requests to the
contact below.

## J. Security

The Service uses TLS/HTTPS in transit, AES-256-GCM encryption at rest, random opaque
single-use tokens, atomic claiming, rate limiting, and automatic expiry/cleanup. No
system is perfectly secure, and absolute security cannot be guaranteed.

## K. Children

The Service is not directed at children under 16. We do not knowingly process personal
data from children. Contact us if you believe a child has used the Service.

## L. Changes to This Policy

We may update this Privacy Policy from time to time. Material changes will be posted
on this page with an updated effective date.

## M. Contact

Questions about this policy or your data: **[privacy@your-domain.tld]**
