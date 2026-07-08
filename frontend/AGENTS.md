# Frontend Guidelines

## Scope

These instructions apply to frontend code under `frontend/`.

## Architecture

- The frontend is plain static HTML, CSS, and JavaScript. Do not introduce a
  build chain, package manager, framework, or bundled assets without explicit
  approval.
- `frontend/static/app.js` is the entry point. Keep it focused on bootstrapping,
  global state, API actions, and render orchestration.
- Put reusable DOM-building code in `frontend/static/components/`.
- Components should receive explicit props and callbacks. Avoid importing or
  mutating global application state from component modules.
- Reuse existing helpers:
  - `api.js` for API calls.
  - `dialogs.js` for modal prompts and confirmations.
  - `tooltip.js` for tooltip behavior.
  - `formatters.js` for display formatting and snapshot parsing.

## UI And Compatibility

- Keep existing CSS class names and DOM semantics stable unless the visual or
  interaction change is intentional.
- Preserve keyboard support and ARIA attributes for menus, dialogs, tooltips,
  and action buttons.
- Buttons that must remain disabled outside global loading should set
  `data-disabled-when-idle`; `setLoading()` depends on that convention.
- When changing module imports or static entry points, update query-string
  versions in the relevant imports or `index.html` to avoid stale browser
  module cache during local use.

## Verification

- Run `node --check` for every edited JavaScript file.
- For component import changes, run a lightweight ESM import check when
  practical.
- For layout, hover, keyboard, dialog, or menu behavior changes, perform browser
  visual/interaction verification when the browser environment is available;
  otherwise report that it was not run.
- CSS-only changes should at minimum pass `git diff --check`.
