# Security Policy

## Supported Versions

| Version | Supported |
| ------- | --------- |
| `main`  | ✅ |
| Latest release | ✅ |

## Reporting a Vulnerability

Email [security@bengobox.com](mailto:security@bengobox.com) with subject `SECURITY: Food Delivery Backend`. Include:

- Description and impact assessment
- Steps to reproduce (or proof of concept)
- Affected versions / commit SHA
- Suggested remediation, if known

We will acknowledge within 48 hours and provide an initial response within 5 business days.

## Responsible Disclosure Guidelines

- Do not publicly disclose vulnerabilities before a fix is released
- Avoid accessing, modifying, or deleting data without permission
- Do not perform tests that degrade availability or integrity of production systems

## Patch Process

1. Security team triages and assigns priority/CVSS score
2. Fix developed on private branch, reviewed by security peers
3. Mitigation deployed via CI/CD + ArgoCD
4. Advisory published in [`CHANGELOG.md`](CHANGELOG.md) and internal channels

Thank you for helping us keep our customers safe.
