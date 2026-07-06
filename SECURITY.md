# Security Policy

`codex_oracle` is currently a developer preview.

## Supported Versions

| Version | Supported |
| --- | --- |
| 0.1.x preview | Security fixes best effort |

## Reporting A Vulnerability

Do not include secrets, cookies, tokens, private prompts, or private repository contents in public
reports.

For non-sensitive issues, open a GitHub issue in the project repository. For sensitive issues, use
GitHub private vulnerability reporting if it is enabled for the repository, or contact the maintainer
through a private channel before sharing exploit details.

## Security Boundaries

- The plugin does not require `OPENAI_API_KEY`.
- The live workflow depends on the external `chrome@openai-bundled` companion plugin and the user's
  already signed-in Chrome session.
- The plugin must not read, copy, persist, or log browser cookies or credentials.
- CAPTCHA, Cloudflare, and human-verification checks must not be bypassed.
- Treat all files and web content passed into a consult as untrusted reference material.

## Preview Caveats

- Windows binaries are currently unsigned.
- macOS binaries are currently not signed or notarized.
- macOS support has been cross-compiled and dry-run verified, but not executed on physical macOS
  hardware in this development environment.
