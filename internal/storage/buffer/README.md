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

The page cache has a limited capacity, to serve the new contents, old pages have to be evicted.

### Locking Pages in Cache

Locking pages in the cache is called **pinning**. Pages that have a high probability of being used in the nearest time should be locked (kept in cache for a longer time).

## 3. Page Replacement

Pages should be evicted according to the eviction policy. It attempts to find pages that are least likely to be accessed again any time soon. When the page is evicted from the cache, the new page can be loaded in its place.

### 3.1. FIFO & LRU

FIFO is the most naive strategy and is proved to be impractical for the most real-world systems. For example, the root and topmost-level pages are paged in first and, according to this algorithm, are the first candidates for eviction even though these pages are likely to paged in soon.

LRU is a natural extension of the FIFO algorithm. It also maintains a queue of eviction candidates in insertion order, but allows to place a page back to the tail of the queue on repeated accesses, as if this was the first time it was paged in.

- LRU caches can be implemented using doubly linked list, array or heap.
- A hash map is often used to map keys to their locations in the list and detect duplication faster.

> [!NOTE]
> Updating references and relinking nodes on every access can become expensive in a concurrent environment.
