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
For questions about agent use in this repo, open an issue or contact the maintainers listed in the repository.
