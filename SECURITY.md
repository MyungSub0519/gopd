# Security reporting

GoPD is under development. Resource limits are part of its parser contract, but
they do not represent a complete process-memory or execution-time sandbox.
See [resource limits](docs/resource-limits.md) for their scope.

If private vulnerability reporting is enabled on this repository, use the
Security tab's **Report a vulnerability** entry. If that entry is unavailable,
open an issue requesting a private reporting channel without including the
triggering document or sensitive details. A published release requires a
maintainer-confirmed private contact channel.

A useful report includes the commit/version, Go version and architecture,
configured limits, the observed behavior, and a minimized synthetic reproducer.
Do not attach private PDF documents or personal information.

There is no published supported-version or response-time policy yet. Those
policies must be set before a stable release.
