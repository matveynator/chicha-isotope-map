# DuckDB performance notes

Slow startups on large `.duckdb` files usually come from replaying the write-ahead log and scanning the whole file to rebuild metadata. The server already caps connections to one and bumps `PRAGMA threads` and `PRAGMA checkpoint_threshold` at startup so bulk imports stay CPU-bound instead of stalling on checkpoints. You can speed up future launches by trimming the log and compacting the database after big imports.

## Cut startup lag
1. Stop the server cleanly so the file is closed without an in-progress transaction.
2. Run maintenance once on the data file:
   ```bash
   duckdb /backup/pelora.org.duckdb \
     "PRAGMA optimize; CHECKPOINT; VACUUM;"
   ```
   * `PRAGMA optimize` refreshes statistics so bounding-box filters stay selective.
   * `CHECKPOINT` flushes the transaction log so the next boot does not have to replay gigabytes.
   * `VACUUM` compacts old segments and can shrink the file so sequential reads finish sooner.
3. Restart the server; the progress lines should advance much faster.

## Keep DuckDB for interactive queries
Parquet is great for archival snapshots, but map tiles query by geography and time. The `.duckdb` file keeps zone maps and statistics close to the data, so bounding-box filters skip large swaths of pages. Plain Parquet files would need to be re-opened and filtered on every request, losing those indexes and usually hitting the disk harder. Use Parquet exports for cold backups; keep the live node on the native DuckDB file for fastest interactive reads.

## When you still need Parquet
If you want a secondary Parquet mirror for offline analytics, export directly from the live file without stopping the node:
```bash
duckdb /backup/pelora.org.duckdb "EXPORT DATABASE 'parquet-export' (FORMAT PARQUET);"
```
The export is append-only and does not interfere with the running server. Consumers can read the Parquet directory, while the main map keeps its optimized `.duckdb` store.
