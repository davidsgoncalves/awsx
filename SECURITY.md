# Security Policy

## Design

AWSX is built to avoid handling secrets directly:

- It never stores or prints AWS credentials, session tokens, SSO tokens,
  passwords, or MFA codes.
- Authentication uses the official AWS CLI cache. AWSX reads
  `~/.aws/config`, `~/.aws/credentials`, and `~/.aws/sso/cache`; it delegates
  login to `aws sso login` and sessions to `aws ssm start-session`.
- AWSX does not modify `~/.aws/config`.
- Debug logging (`AWSX_DEBUG=true`) never emits credentials or tokens — only
  metadata such as profile, region, instance, and error code.

## Reporting a Vulnerability

If you find a security issue, please report it privately rather than opening a
public issue. Use GitHub's "Report a vulnerability" (Security Advisories) on
the repository, or contact the maintainer directly.

Please include:

- A description of the issue and its impact
- Steps to reproduce
- Any relevant version/environment details (without credentials or tokens)

You can expect an acknowledgement and a plan for a fix or mitigation.

## Supported Versions

AWSX is pre-1.0. Security fixes target the latest released version.
