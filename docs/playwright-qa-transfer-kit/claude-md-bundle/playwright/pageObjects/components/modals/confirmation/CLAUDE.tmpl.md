# modals/confirmation - Confirm/cancel dialog

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **confirmationModal.ts** - `ConfirmationModal`. Generic confirm/cancel dialog. Methods:
  `validateVisible()`, `validateNotVisible()`, `confirmAction()`, `cancelAction()`, `checkIsVisible()`.
  Targets `confirmation-modal-button-confirm` / `-cancel`.

## How to use it
Used inside destructive flows (delete/archive) by the table classes. It only clicks; the CALLER waits
for the mutation and asserts the end state (row gone, count changed).

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
