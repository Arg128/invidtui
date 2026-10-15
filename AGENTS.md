# AGENTS

This repository welcomes the responsible use of software agents (AI assistants, automated bots, CI agents) to help with development, code review, testing, and maintenance. This document explains which agents are permitted, how they may be used, and the rules contributors and maintainers should follow when using agents against this repository.

## Purpose

Agents can help speed development and reduce busywork, but they must not be used as a substitute for human review or to introduce changes that might compromise security, privacy, or code quality. Use agents to assist — not to replace — human judgement.

## Allowed agents and automation

- Human-facing AI assistants (example: GitHub Copilot, ChatGPT) for local development and generating suggestions.
- Repository automation bots (example: Dependabot) and CI systems (GitHub Actions) that run checks, builds, and tests.
- Custom automation owned by repository maintainers (documented in this repo) that performs maintenance tasks.

## Prohibited or restricted uses

- Do not use agents to automatically commit or merge changes without human review.
- Avoid using agents to handle secrets, credentials, or private keys. Never include secrets in prompts or commit them to the repo.
- Do not rely on agent output as authoritative: agent-generated code must be reviewed and tested by a human maintainer before merging.

## Contribution workflow with agents

1. When an agent assists with a change, add a short note in the PR description describing how the agent was used (which agent, what prompts or templates were used, and why).
2. Treat agent suggestions as drafts. The contributor must:
   - Ensure code follows the repository's style and patterns.
   - Run tests and linters locally or via CI.
   - Verify there are no inadvertent license, security, or privacy regressions.
3. Maintain the same review standards for agent-assisted changes as for hand-written changes.

## Prompts and reproducibility

- Keep prompts short, deterministic, and reproducible when they are important for the change. Prefer saving any complex prompt templates in the repo (for example in `.github/agent-templates/` or a separate `agents/` directory) so reviewers can reproduce results.

## Security and secrets

- Never include API keys, credentials, or other secrets in prompts or committed files.
- If an agent reveals a secret in generated output or a suggested change, remove it and rotate the secret immediately.

## Tests, CI and verification

- All changes, including agent-assisted edits, must pass CI (build, tests, linters) before merge.
- If an agent introduces a change that expands the test surface (for example: new dependencies), the contributor must justify it and add tests where appropriate.

## Ownership and accountability

- The contributor who opens a PR is accountable for the change, regardless of agent involvement.
- Maintainers reserve the right to revert or request changes to any agent-assisted commit.

## Adding new automation or agents

If you want to add an automated agent (bot, scheduled job, GitHub App) that operates on this repository, open an issue describing:
- The agent's purpose and ownership.
- What it will change or check.
- Frequency and trigger conditions.
- Permissions required.

Maintainters will review and approve the automation before it is added.

## Contact / Maintainers

For questions about agent use in this repo, open an issue or contact the maintainers listed in the repository.
