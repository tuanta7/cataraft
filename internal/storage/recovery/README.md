# Recovery

This package implements writer with recovery strategies following Part 2, "Recovery" of Chapter 5 of the book "Database Internals".

## 1. Crash Recovery

Changes to disk are not instantaneous, and hardware can fail at any moment. A crash mid-write leaves storage in a partial state, neither the old version nor the new one is intact. Two properties are required of any reliable storage system:

- **Atomicity**: A write either fully completes or does not happen at all.
- **Durability**: Once committed, data survives crashes. At least one `fsync` syscall must be called, which forces data from OS buffers to physical disk.

### 1.1. Naive Approaches

Reference: [From Files To Databases](https://build-your-own.org/database/01_files)

Atomicity means different things in different contexts.

- **Readers-Writer Atomic**: Concurrent readers observe either the old state or the new state, never a partial mix of both.
- **Power-Loss Atomic**: The new state is either fully reflected on disk or not at all, even if power is lost mid-operation.

#### In-Place Updates

In-place updates involve writing directly over existing data on disk. This approach is considered unsafe because a crash mid-write destroys the original data without completing the new version. Atomicity is not guaranteed, and no recovery path exists.

#### Atomic Renaming

A temporary file is written with the new data, then renamed over the original. On most filesystems, the rename operation itself is atomic at the OS level, readers will see either the old file or the new one, never a mix.

Replacing data atomically by renaming files is only readers-writer atomic, it is not power-loss atomic or durable.

- A clean new version requires both the file data and the directory entry to be written to disk.
- The directory entry (metadata record that maps a filename to the underlying file data) update produced by the rename may not be flushed to disk before a crash occurs. What the disk contains after a crash is unpredictable.
- An extra fsync on the parent directory is required after the rename for durability.

### 1.2. Copy-on-write

Copy-on-write atomically switches everything to the new version.

### 1.3. Double-write (with WAL)

## 2. Write-Ahead Log & Checksum

Write-Ahead Log (WAL) or Append-Only Log is an append-only auxiliary disk-resident structure used for crash and transaction recovery.
