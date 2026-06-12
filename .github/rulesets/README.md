# GitHub Rulesets

This directory stores GitHub repository ruleset exports for manual import or
API-based sync.

- `Git-Flow-Branch-Naming.json`: protects `main` and `dev` from deletion and
  non-fast-forward updates, and requires pull requests for changes.

GitHub does not automatically apply files in this directory. Import the JSON in
repository settings or sync it with the GitHub Rulesets API.
