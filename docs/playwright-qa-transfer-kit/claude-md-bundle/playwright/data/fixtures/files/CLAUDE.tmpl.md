# playwright/data/fixtures/files - Binary upload assets

Binary files used by upload tests. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **<video file>.mp4** - a ~3 MB MP4. The single video fixture used for video and media-upload tests;
  its path is wired into `support/utils.ts` (`generateVideoFileForUpload`).

## Conventions & gotchas
- Only one video fixture exists - all video tests share it. If you need another format/size, add a file
  here and extend the util helper rather than repointing this one.
- Image upload fixtures live elsewhere (`<image fixtures folder>`), not in this folder.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
