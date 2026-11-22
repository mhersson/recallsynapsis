# AGENTS.md

## The bar

The owner's words, which bind every change:

- "I demand super tight, simple and efficient code. We do not do
  over-engineering, we do not write tons of code to remedy far fetched edge
  cases that will never happen under normal usage, however we do not take
  lightly on security. The goal is always the best product possible.""

## Conventions

- Conventional commits with a scope: `type(scope): summary`, with bullet lists
  in bodies.
- No attribution lines, and no session links, task or milestone numbers, or plan
  references in commits, branch names, code comments or documentation.
- No em-dashes anywhere.
- Tests exercise behaviour and never assert on prose.
- One branch per pull request, one commit per logical change. The
  orchestrating session may push feature branches and open pull requests.
  Never push to `main`, and never merge without the owner's word.

## Documentation

- Documentation must always be true: every statement matches the code at HEAD,
  and a change that makes a doc line false updates it in the same commit.
- Document the current state - what exists now and why, not how it got here.
- Comments explain only non-obvious decisions, constraints, safety invariants,
  or workarounds. Do not narrate what code does or record change history; git
  holds history. Keep comments to one or two tight lines unless a longer
  explanation is genuinely necessary. Rewrite or delete if you find comments
  that don't follow this rule.
