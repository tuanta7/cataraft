# Cataraft

Cataraft is a simple distributed database project written in Go. The first phase focuses on building a small storage engine with crash recovery and replication primitives rather than a full SQL database.

The initial system is intended to support:

- A B+ tree storage/index structure
- Double-write protection for safer page persistence
- Write-ahead logging (WAL) for crash recovery
- An LRU-managed buffer pool
- Raft consensus for horizontal scaling and replicated state

The short-term target is a minimal but coherent database core that can safely persist data on one node and evolve into a replicated multi-node system.

## High-level Architecture

- The B+ tree never touches the disk adapter directly; it only speaks to the Buffer Pool.
- The WAL and Double-Write Buffer both reach the Disk Adapter but through separate file handles

```mermaid
flowchart TD
    client[Client / CLI]
    query[Query / Execution]
    bptree[B+ Tree access/index path]
    buffer[Buffer Pool Layer<br/>CoreBuffer + LRU eviction policy]
    flush[Dirty Page Flush]
    reads[Page Cache Reads]
    wal[Recovery / WAL<br/>append log + fsync]
    dw[Double-Write Buffer<br/>temporary safe page copy]
    disk[Disk Adapter<br/>page/file primitives]
    machine[Machine Disk<br/>data files + wal log]

    client --> query
    query --> bptree
    bptree --> buffer
    buffer --> flush
    buffer --> reads
    flush --> wal
    flush --> dw
    reads --> disk
    wal --> disk
    dw --> disk
    disk --> machine
```

### Write Path

```mermaid
sequenceDiagram
    participant C as Client
    participant T as B+ Tree
    participant B as Buffer Pool
    participant W as WAL
    participant X as Double-Write
    participant D as Disk Adapter
    participant R as Raft

    C->>T: write request
    T->>B: load or create target page
    B->>W: append page mutation
    W-->>B: return LSN
    B-->>T: page updated in memory
    T->>R: replicate command
    B->>W: ensure WAL is durable before flush
    W->>D: fsync WAL
    B->>X: copy page into double-write area
    X->>D: write + fsync double-write buffer
    B->>D: write dirty page
    D-->>B: page persisted to primary location
```

### Crash Recovery With Double-Write

```mermaid
sequenceDiagram
    participant S as Startup / Recovery
    participant D as Disk Adapter
    participant X as Double-Write Area
    participant P as Primary Data Pages
    participant W as WAL

    S->>D: open database files
    S->>X: scan double-write buffer
    X-->>S: pages that were mid-flush before crash
    S->>P: restore any torn or incomplete primary pages
    S->>W: scan WAL records
    W-->>S: redo records after last durable page state
    S->>P: replay remaining page updates
    S-->>D: database returns to a consistent state
```

### Replication View

```mermaid
flowchart LR
    subgraph A[Node A]
        araft[Raft node]
        awal[local WAL]
        abuffer[buffer]
        adisk[disk]
        araft --> awal --> abuffer --> adisk
    end

    subgraph B[Node B]
        braft[Raft node]
        bwal[local WAL]
        bbuffer[buffer]
        bdisk[disk]
        braft --> bwal --> bbuffer --> bdisk
    end

    subgraph C[Node C]
        craft[Raft node]
        cwal[local WAL]
        cbuffer[buffer]
        cdisk[disk]
        craft --> cwal --> cbuffer --> cdisk
    end

    araft <-->|Raft RPCs| braft
    braft <-->|Raft RPCs| craft
    araft <-->|Raft RPCs| craft
```

## Getting Started

```bash
go run ./cmd/cataraft
```

The process expects a data directory through `CATA_DATA`. On Linux, it falls back to `/var/lib/cataraft` when the variable is not set.

```bash
CATA_DATA=/tmp/cataraft go run ./cmd/cataraft
```

## Codex

- [Custom instructions with AGENTS.md](https://developers.openai.com/codex/guides/agents-md)

## References

- [Build Your Own Database From Scratch in Go](https://build-your-own.org/database/)
- [Database Internals](https://www.amazon.com/Database-Internals-Deep-Distributed-Systems/dp/1492040347)
- [Designing Data-Intensive Applications](https://www.amazon.com/Designing-Data-Intensive-Applications-Reliable-Maintainable/dp/1449373321)
- [Raft Consensus Algorithm](https://raft.github.io/)
