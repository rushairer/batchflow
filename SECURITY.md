# Security Policy

## Supported Versions

Security fixes are provided for the latest released minor version and the `main` branch.

| Version | Supported |
|---|---|
| latest release | Yes |
| older releases | Best effort |

## Reporting a Vulnerability

Do not open a public GitHub issue for suspected security vulnerabilities.

Report privately through GitHub Security Advisories:

<https://github.com/rushairer/batchflow/security/advisories/new>

If GitHub Security Advisories are unavailable, contact the maintainers privately and include:

- affected BatchFlow version or commit
- Go version and operating system
- database/backend involved, if applicable
- minimal reproduction steps
- expected impact and any known workaround

## Handling Process

The maintainers will triage reports as follows:

1. Acknowledge receipt within 7 days when possible.
2. Confirm scope, severity, and affected versions.
3. Prepare a fix and tests privately when needed.
4. Publish a release and advisory after the fix is available.

## Security Expectations

BatchFlow integrations must not expose raw row payloads, SQL args, Redis keys, HTTP bodies, credentials, tokens, or user identifiers in logs or Prometheus labels. Use fingerprints, counts, low-cardinality reason labels, and redacted structured attributes instead.
