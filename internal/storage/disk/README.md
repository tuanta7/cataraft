# Write Techniques

Changes to disk are not instantaneous, and hardware can fail at any moment. A crash mid-write leaves storage in a partial state, neither the old version nor the new one is intact. Two properties are required of any reliable storage system:

- **Atomicity**: A write either fully completes or does not happen at all.
- **Durability**: Once committed, data survives crashes. At least one `fsync` syscall must be called, which forces data from OS buffers to physical disk.

## 1. Naive Approaches

Reference: [From Files To Databases](https://build-your-own.org/database/01_files)

Atomicity means different things in different contexts.

- **Readers-Writer Atomic**: Concurrent readers observe either the old state or the new state, never a partial mix of both.
- **Power-Loss Atomic**: The new state is either fully reflected on disk or not at all, even if power is lost mid-operation.

### In-Place Updates

In-place updates involve writing directly over existing data on disk. This approach is considered unsafe because a crash mid-write destroys the original data without completing the new version. Atomicity is not guaranteed, and no recovery path exists.

### Atomic Renaming

A temporary file is written with the new data, then renamed over the original. On most filesystems, the rename operation itself is atomic at the OS level, readers will see either the old file or the new one, never a mix.

Replacing data atomically by renaming files is only readers-writer atomic, it is not power-loss atomic or durable.

- A clean new version requires both the file data and the directory entry to be written to disk.
- The directory entry (metadata record that maps a filename to the underlying file data) update produced by the rename may not be flushed to disk before a crash occurs. What the disk contains after a crash is unpredictable.
- An extra fsync on the parent directory is required after the rename for durability.

## 2. Copy-on-Write ()

Copy-on-Write atomically switches everything to the new version. CoW is a prevention strategy, it never allows a corrupt state to be visible at all.

- First write new version to new block
- Then atomically update the pointer (single sector write)
- If crash happens before pointer swap, old page is still valid. No torn page possible
- If crash happens after pointer swap, new page is fully written. Also fine

A **failed write** in CoW means the pointer swap never happened, which leaves the database exactly as it was before.

> [!NOTE]
> CoW already provides crash safety at the page level, so WAL is not required for correctness. If WAL is still present, it is usually introduced for operational or performance reasons (faster recovery, change-data-capture, etc.).

### Considerations

- High write amplification on deep trees

## 3. Double-Write Buffer (MySQL)

Reference: [Innodb Double Write](https://www.percona.com/blog/innodb-double-write/)

Double-Write Buffer is used by database engines like MySQL InnoDB to solve the torn page problem during crash or power loss. DW is a repair strategy, it fixes the page after crash, then re-applies changes.

- First write full page to doublewrite buffer (sequential I/O + fsync to ensure durability)
- Then write page to its actual location (in-place edit, crash possible mid-write leads to torn page)
- Recovery if torn page detected at startup

DW has a more complex failure surface because it has more steps. A write can fail at several distinct points:

- Failure before the DW buffer is fully written
- Failure after DW buffer written, before actual page write begins
- Failure mid-way through actual page write

### DW's Recovery Problem

After a crash, the doublewrite buffer has the page as it was when the flush began, not the latest committed state.

- DW is used to restore the page to a clean state
- WAL replays the committed transactions on top of the restored page

## 4. Full Page Writes (PostgreSQL)
