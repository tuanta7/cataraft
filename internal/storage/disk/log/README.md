# Recovery with Write-Ahead Log

This package implements writer with recovery strategies following Part 2, "Recovery" of Chapter 5 of the book "Database Internals".

Write-Ahead Log (WAL) or Append-Only Log is an append-only auxiliary disk-resident structure used for crash and transaction recovery.

## Checksum