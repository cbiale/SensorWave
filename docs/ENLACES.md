# Enlaces de referencia

## Para las aplicaciones

- A Comprehensive Guide to Zap Logging in Go: https://betterstack.com/community/guides/logging/go/zap/
- SQL Has Problems. We Can Fix Them: Pipe Syntax In SQL: https://research.google/pubs/sql-has-problems-we-can-fix-them-pipe-syntax-in-sql/
- Big Data is Dead: https://motherduck.com/blog/big-data-is-dead/
- Understanding Apache Arrow Flight: https://www.dremio.com/blog/understanding-apache-arrow-flight/
- Faster reading from the Lakehouse to Python with DuckDB/ArrowFlight: https://www.hopsworks.ai/post/python-centric-feature-service-with-arrowflight-and-duckdb (Ejemplo de uso de DuckDB y ArrowFlight)
- SlateDB: An Embedded Storage Engine Built on Object Storage: https://materializedview.io/p/slatedb-an-embedded-storage-engine
- Test de concurrencia entre DuckDB y Sqlite: https://github.com/breckcs/duckdb_sqlite_scanner_concurrency_tests
- Mochi-Mqtt: https://github.com/mochi-mqtt/server
- Hash tables in ClickHouse and C++ Zero-cost Abstractions: https://clickhouse.com/blog/hash-tables-in-clickhouse-and-zero-cost-abstractions
- Time-series and analytical databases walk into a bar: https://questdb.io/blog/2024/10/28/time-series-analytic-database-p99-andrei/
- Go I/O Closer, Seeker, WriterTo, and ReaderFrom
: https://victoriametrics.com/blog/go-io-closer-seeker-readfrom-writeto/ (Tiene lecturas sobre go)
- Parquet pruning in DataFusion
: https://blog.haoxp.xyz/posts/parquet-to-arrow/
- Capacidades funcionales en Go: https://www.bytesizego.com/blog/10-years-functional-options-golang

## Aplicaciones de referencia

- SlateDB: https://github.com/slatedb/slatedb-go
- Tonbo: https://github.com/tonbo-io/tonbo
- Leanstore: https://github.com/leanstore/leanstore
- ArcticDB: https://github.com/man-group/ArcticDB
- Monotone: https://github.com/monotone-studio/monotone

## Ayuda

- Obtener compilador de FlatBuffers: 
    ```
    sudo apt-get install flatbuffers-compiler
    ```
- Compilar un archivo `.fbs`: 
    ```
    flatc --go payload.fbs
    ```
- Biblioteca de Docker en go: 
    ```
    go get github.com/docker/docker/client
    go get github.com/docker/docker/api/types
    go get github.com/docker/docker/api/types/container
    ```

## Referencias compresores

https://yifan-online.com/en/km/article/detail/4819
https://codezup.com/go-data-compression-real-world-uses/
https://github.com/dgryski/go-tsz/tree/master
