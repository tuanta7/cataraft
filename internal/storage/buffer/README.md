# Page Cache (Buffer Pool)

This package implements a page cache following Part 1, "Buffer Management" of Chapter 5 of the book "Database Internals".

The page cache serves as an intermediary between the disk and the rest of the storage engine to reduce the number of accesses to persistent storage (disk). If an already cached page is requested, its cached version is returned.

> [!CAUTION]
> Operating systems have the concept of a page cache, too. They use unused memory segments to transparently cache disk contents to improve performance of I/O syscalls.
>
> Many database systems open files using O_DIRECT flag. This flag allows I/O system calls to bypass the kernel page cache, access the disk directly, and use database-specific buffer management.

## 1. Caching Semantics

Cached pages available in memory can be reused under the assumption that no other process has modified the data on disk.

- This synchronization is a one-way process: from memory to disk, and not vice versa
- When the storage engine accesses the page and it is not yet cached, the cache translates the logical page address (database_id, table_id, etc.) or page number to its physical address, loads its contents in memory, and returns its cached version to the storage engine
- If any changes are made to the cached page, it is said to be dirty until these changes are flushed back on disk.

## 2. Cache Eviction

The page cache has a limited capacity and, sooner or later, to serve the new contents, old pages have to be evicted.

### Locking Pages in Cache

Locking pages in the cache is called **pinning**. Pages that have a high probability of being used in the nearest time should be locked (kept in cache for a longer time).

## 3. Page Replacement

### 3.1. FIFO & LRU
