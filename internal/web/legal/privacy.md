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
"**Service**"). We built the Service to minimize the data we hold about you; this
policy tells you exactly what little there is.

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

Ephemeral Link lets a sender create a "secret" — a short piece of text — behind a
one-time link. Once the recipient opens that link, the secret is deleted from our
servers and cannot be viewed again. We never intend to read, store long-term, or
otherwise access the content of your secrets.

## C. What Data We Process

We keep this as small as possible:

- **Secret content.** The text you submit, stored only until it is read once or its
  expiry time passes, whichever comes first. [Confirm: encrypted at rest? — describe
  how, matching how your deployment actually works.]
- **The secret's link/token.** A random identifier generated for each secret so it can
  be retrieved exactly once.
- **Technical/log data.** Standard web server logs (IP address, timestamp, user agent,
  requested URL) generated automatically when you access the Service, kept only for
  [retention period] for security and abuse-prevention purposes, then deleted.
- **Accounts.** The Service may process administrator/user account names, email
  addresses, role information, authentication metadata, and session data when account
  login or administration features are enabled.

We do not run analytics, advertising, or tracking scripts. Adjust this statement if
that is not true of your deployment.

## D. Legal Basis

Under Art. 6(1) GDPR, we process this data on the following bases:

- **Art. 6(1)(b)** — performance of a contract: storing and delivering the secret you
  ask us to transmit is the core service you're requesting.
- **Art. 6(1)(f)** — legitimate interests: keeping short-lived server logs to secure
  the Service against abuse, and our interest in that is limited and time-boxed.

## E. Retention and Deletion

- A secret is deleted immediately upon being read, or automatically once its expiry
  time is reached — whichever happens first. Once deleted, it cannot be recovered.
- Server logs are deleted after [retention period].
- [Describe encrypted backup retention, or state that no backups of active payloads are kept.]

## F. Cookies and Local Storage

The Service uses strictly necessary session, CSRF, and language-preference cookies as
needed for authentication, security, and language selection. It does not use browser
local storage for advertising or tracking.

## G. Recipients and Hosting

The Service runs on infrastructure operated by [hosting provider name], with servers
located in [data center location]. [If applicable, note that a Data Processing
Agreement (Art. 28 GDPR) is in place.]

We do not sell, rent, or share secret content with third parties. If you configure
SMTP, Microsoft Graph, or S3-compatible object storage, update this section with the
relevant processors and locations.

## H. International Transfers

[Describe whether all processing takes place within the EU/EEA or identify the
transfer mechanism, such as Standard Contractual Clauses.]

## I. Your Rights

If you're in the EU/EEA, you may request access, rectification, erasure, restriction,
data portability, or object to processing based on legitimate interests. You may also
lodge a complaint with the supervisory authority in your country or German state.

Because the Service holds short-lived data and secrets self-destruct, some requests
may already be moot when received. Send requests to the contact below.

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
