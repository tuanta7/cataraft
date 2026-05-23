# Cataraft

Cataraft is a simple distributed key-value store written in Go. The project scope is intentionally narrow:

- B+ tree for ordered key/value storage
- LRU buffer pool for page caching
- Copy-on-write page persistence for crash-safe page updates
- Disk adapter for page and file primitives
- Raft consensus

The project does not include:

- WAL
- double-write/full-page-write 
- SQL/query planning layers
- alternate index types

## Architecture

```text
B+ Tree
  -> LRU Buffer
     -> Copy-On-Write Store
        -> Disk Adapter
```

The B+ tree only talks to the buffer layer. The buffer loads and evicts pages through the copy-on-write store. The copy-on-write store persists page versions into shadow pages and records the latest version in a manifest.

## Persistence And Recovery

Persistence is explicit: writes become durable when the system flushes dirty pages.

Recovery is built on copy-on-write metadata:

- Page updates are written to shadow pages
- The manifest records the latest durable page version
- Startup rebuilds the in-memory page index from the manifest

## Getting Started

```bash
go run ./cmd/cataraft exec SET greeting hello
go run ./cmd/cataraft exec GET greeting
```

## References

- [Build Your Own Database From Scratch in Go](https://build-your-own.org/database/)
- [Database Internals](https://www.amazon.com/Database-Internals-Deep-Distributed-Systems/dp/1492040347)
- [Designing Data-Intensive Applications](https://www.amazon.com/Designing-Data-Intensive-Applications-Reliable-Maintainable/dp/1449373321)
- [Raft Consensus Algorithm](https://raft.github.io/)
