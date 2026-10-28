<<<<<<< HEAD
# Introduction:

This project is a TUI (Terminal User Interface) application that uses MPV to play videos fetched from any Invidious instance. The goal is to provide a free, privacy-focused experience that avoids YouTube tracking and utilizes a minimalist interface—avoiding addictive design patterns or "dopamine-triggering" colors.

# Rules:

1. Do not provide raw code blocks as the primary solution. Instead, provide clear, step-by-step technical instructions telling the programmer exactly what they MUST do to implement the change. Its recommended generate tiny changes :).
2. Find bugs is your work, AI works fine attempting many ways to resolve issues. Your primary responsibility is to proactively identify bugs. When an issue is found, explore and suggest multiple potential resolutions before settling on the best one..
3. Maintain a [./Changelog.md] file. Every implemented change must be documented there immediately. Once the programmer pushes the code to the remote repository, clear the [./Changelog.md] file to start fresh for the next session."
4. If is needed create a new file, first ask.
5. When AI changes finishes, do a summary explaining the new workflow app comparing the old vs new workflows, I'm said, After completing a series of changes, provide a summary comparing the previous workflow to the new workflow to clarify the impact of the updates..


# Contribution workflow with agents
1. When an agent assists with a change, add a short note in the PR description describing how the agent was used (which agent, what prompts or templates were used, and why).
2. Treat agent suggestions as drafts. The contributor must:
3. Ensure code follows the repository's style and patterns.
4. Run tests and linters locally or via CI.
5. Verify there are no inadvertent license, security, or privacy regressions.
6. Maintain the same review standards for agent-assisted changes as for hand-written changes.

# Security and secrets
1. Never include API keys, credentials, or other secrets in prompts or committed files.
2. If an agent reveals a secret in generated output or a suggested change, remove it and rotate the secret immediately.

# Contact / Maintainers
=======
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

>>>>>>> 49d896d6866f43b47dc045bc8f9af3dc79086797
For questions about agent use in this repo, open an issue or contact the maintainers listed in the repository.
