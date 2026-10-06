# Security

## Reporting

Report vulnerabilities privately via GitHub Security Advisories
(“Report a vulnerability” on the repo's Security tab), or email
info@tdvorak.dev. Do not open a public issue for undisclosed
vulnerabilities.

## Threat model

docsync reads repository files and writes only `.env.example` (via
`docsync fix`). It executes no code, makes no network requests except the
optional `npx expo install --check`, and never reads `.env` secrets for
anything beyond key names. If you find a path where docsync could leak
file contents or execute unexpected code, that's a reportable issue.
