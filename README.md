# GoLang DataBase (aka golangdb)

![License](https://img.shields.io/badge/license-MIT-blue)
![Go Version](https://img.shields.io/badge/go-1.25.x-blue)
![Status](https://img.shields.io/badge/status-educational-orange)

A small, self-contained in-memory key-value database with a write-ahead log (WAL) and snapshotting, plus an HTTP backend exposing a minimal authenticated CRUD API. A small Python CLI client demonstrates basic usage.

This repository is intended as an educational / lightweight storage + API stack. It is not a production-grade database — see "Limitations & Drawbacks" sections below.

---

Table of contents
- Quick Start
- Features
- Installation
- Project layout
- Database core — architecture and services
    - Record format
    - Persistence lifecycle (WAL, snapshot)
    - Public methods and behaviors
    - Limitations and failure modes
- Backend (HTTP server) — routes, auth, examples
    - Environment
    - Endpoints (sign-up, login, protected CRUD)
    - JWT and auth flow
    - Limitations and gotchas
- Python client (client.py) — brief usage
- Development / tests
- Troubleshooting & tips
- Contributing ideas & potential improvements

---

Quick start

Prerequisites:
- Go toolchain (tested with Go 1.25.x)
- Python 3 (for the included client) and requests library (pip install requests)

1. Create a .env file (required):
    - Example:
      JWT_SECRET=supersecretjwtkey
      PORT=8080

2. Build and run:
    - Build: go build ./...
    - Run: ./golangdb   (or: go run ./)
      The server listens on PORT (defaults to 8080).

3. Use the Python client or curl examples below to interact with the server.

---

Features

- In-memory key/value storage backed by WAL and snapshotting for durability
- Minimal query DSL for Insert / Select / Delete with simple where-clauses
- Auto-increment metadata per-table
- HTTP backend with JWT auth, sign-up/login and protected CRUD endpoints
- Small Python CLI client demonstrating the API

---

Installation

Prerequisites (repeated for clarity):
- Go 1.25.x toolchain
- Python 3 and pip (for client)
- Create .env with at least JWT_SECRET

Commands:
- Build & run:
    - go build ./...
    - ./golangdb
- Or:
    - go run .

- Python client:
    - pip install requests
    - python client.py

---

Project layout (important files)
- main.go — program entrypoint; loads .env, initializes DB, starts server, handles graceful shutdown.
- database/db_core.go — low-level database core: in-memory map, WAL, snapshot, record IO, concurrency.
- database/table_and_schemas.go — higher-level DB wrapper (DB) with Insert/Select/Delete queries; auto-increment metadata; JSON storage semantics.
- database/helpers.go — where-clause evaluation, type normalization, allowed value types.
- server/server.go — chi router, middleware wiring, server lifecycle.
- server/handlers.go — HTTP handlers for sign-up, login, create/select/delete.
- server/jwt.go — JWT middleware, GenerateJWT, AdminOnly middleware.
- server/users_sighned_up.go — bcrypt password hashing utilities.
- client.py — Python CLI demonstrating API usage.

---

Database core — architecture and services

Overview
- The Database core (database.Database in database/db_core.go) provides an in-memory map[string][]byte as the working dataset.
- All rows are stored as JSON blobs, keyed by "table:id" keys (the DB wrapper builds those keys).
- Persistence is implemented using a write-ahead log (WAL) and periodic snapshotting of the full in-memory state to a snapshot file.

Key concepts
- WAL path: ./db/wal.log
- Snapshot (database file) path: ./db/database.db
- WAL size threshold: WalSizeLimit (10 MiB by default). When WAL size exceeds this limit during an apply, snapshot is triggered.
- Concurrency: internal sync.RWMutex protects the in-memory map. Public methods use appropriate locks:
    - Get and ScanPrefix use RLock.
    - Set and Delete use Lock.
    - snapshot is invoked while holding the write lock (applyHelper calls snapshot under lock).

Record format (on disk)
- Each WAL record is written as:
    - uint32 record length (big-endian)
    - byte op ('S' for set/save, 'D' for delete)
    - uint32 key length
    - uint32 value length (0 for delete)
    - key bytes
    - value bytes (JSON marshalling of the stored row)
- Snapshot file format:
    - Sequence of snapshot records: uint32 key length, uint32 value length, key bytes, value bytes

Persistence lifecycle
- OpenDB(dbPath, walPath, walSizeLimit) initializes files and:
    1. loadSnapshot — loads snapshot file entries into memory (if exists).
    2. replayWal — reads WAL records and applies them to memory.
- Set/Delete call applyHelper, which:
    1. serializes and writes a WAL record
    2. fsyncs (walFile.Sync())
    3. applies the change in memory
    4. checks WAL size and triggers snapshot if limit exceeded
- snapshot writes a temp snapshot file, syncs, renames it into place, reopens DB file, truncates WAL by reopening WAL with O_TRUNC|O_APPEND, and persists the new state.

Public core API (low-level)
- Get(key string) ([]byte, bool) — returns a copy of the value if present.
- Set(key string, val []byte) error — writes WAL + updates memory; triggers snapshot if needed.
- Delete(key string) error — writes WAL with delete and removes key from memory.
- ScanPrefix(prefix string) map[string][]byte — returns copies of key/values whose keys begin with prefix.
- Close() error — syncs and closes WAL and DB files.

Higher-level DB wrapper
- DB type (database/table_and_schemas.go) provides:
    - Insert() -> InsertQuery: Table(name).Values(map[string]any).Exec() / ExecAndReturnID()
        - Generates auto-increment ID stored in "__Meta__:<table>:next_id" key.
        - Stores row as JSON under "<table>:<id>".
        - Allowed value types: string, int, int64, float64, bool.
    - Select() -> SelectQuery: Table(name).Where(...).All() — scans prefix, decodes JSON rows, filters with WhereClause (operators "=", "!=", "<", ">").
    - Delete() -> DeleteQuery: Table(name).Where(...).Exec() — scans prefix and deletes matching rows or all rows if no where.

Where-clause and type handling
- WhereClause supports string, numeric, and boolean comparisons.
- Normalization converts json.Number, int, int64, float64 to float64 for numeric comparison.
- String comparisons are lexicographic.
- json.Decoder uses UseNumber() for preserving numbers as json.Number during decode, but comparisons convert to float64 — possible precision loss.

Limitations and failure modes (what can go wrong)
- Single-process, single-node only. No clustering, replication, or leader election.
- No transactions or multi-key atomic operations. Each Set/Delete writes an individual WAL entry — no multi-statement atomicity.
- WAL / snapshot durability edge-cases:
    - applyHelper writes WAL and fsyncs before applying to memory, which helps durability, but if the process crashes during snapshot, snapshot and WAL rotation could leave files in a state requiring replay; OpenDB handles replay but incomplete/partially-corrupted records will return an error.
    - Snapshot replaces the DB file via rename; if rename fails, you can be left with old files — code reports an error and return to caller.
- WAL growth / snapshot cost:
    - Snapshot rewrites the entire dataset to disk and reopens files while holding locks; snapshot can be expensive for large datasets and will block writes during execution.
- Memory + scanning cost:
    - ScanPrefix iterates the entire in-memory map — large datasets will increase memory and scanning latency.
    - There is no secondary index; all queries are prefix-based by design ("table:" keys).
- Limited allowed value types:
    - Only string, int, int64, float64, bool are allowed. Complex/nested JSON types (lists, objects) are not permitted.
- Numeric normalization:
    - json.Number/ints are normalized to float64 for comparisons, which may cause precision loss for very large integers.
- Key namespace collisions:
    - Keys are simple strings composed by the DB wrapper (e.g., "user:123:contacts"). Clients and server must follow the same naming to avoid collisions.
- No per-row schema validation:
    - Values are stored as JSON blobs; higher-level logic enforces allowed types but no strong schema.

---

Backend (HTTP server)

Overview
- Implemented using chi router and go-chi middleware.
- JWT-based authentication (server/jwt.go). Tokens are signed with HS256 using JWT_SECRET from environment (.env).
- Passwords hashed with bcrypt.
- Routes:
    - Public: POST /sign-up (register), POST /login (obtain JWT)
    - Protected (JWT middleware required): POST /create, GET /get, DELETE /delete
    - Admin-only group: GET /admin/getall (calls same select handler but admin can query across users)

Environment variables
- JWT_SECRET (required) — used to sign tokens. If not set, JWT parsing will fail.
- PORT (optional) — server listens on this port (default "8080").

Payload shapes and examples

Sign up
- Request: POST /sign-up
    - JSON: { "email": "user@example.com", "password": "secret" }
- Response: 200 OK
    - JSON: { "token": "<JWT>" }
- Notes: If email already exists the server returns 400.

Login
- Request: POST /login
    - JSON: { "email": "user@example.com", "password": "secret" }
- Response: 200 OK
    - JSON: { "token": "<JWT>" }

Protected routes — Authorization
- All protected endpoints require header:
  Authorization: Bearer <token>

Create / Insert
- Request: POST /create (protected)
    - JSON: { "table": "contacts", "values": { "name": "Bob", "phone": "123", "id": 5? } }
        - If "id" is missing the DB will generate an auto-increment id (InsertQuery.nextID uses a meta key).
    - Response: 201 Created on success
- Notes:
    - The server prefixing mechanism stores data under keys like "user:<userID>:<table>" to isolate user data.
    - Valid value types for fields: string, number (int/float), boolean.

Select / Get
- Request: GET /get (protected) — this server accepts a GET with JSON body (non-standard but implemented).
    - JSON: { "table": "contacts", "where": { "field": "age", "op": ">", "value": 30 } }
    - Response: 200 OK
        - JSON: [ {row1}, {row2}, ... ]
- Notes:
    - If the authenticated user is an admin (is_admin true in token claims), the server sets prefix to "user:" allowing cross-user queries (use with caution).
    - GET with body: many HTTP clients and intermediaries may not support sending or routing GET requests with a JSON body; consider using POST for complex queries in real-world usage.

Delete
- Request: DELETE /delete (protected)
    - JSON: { "table": "contacts", "where": { "field": "id", "op": "=", "value": 7 } }
    - Response: 204 No Content
    - If "where" is omitted, deletes all rows in the table for the current user (dangerous).

Admin-only route
- GET /admin/getall (protected + AdminOnly middleware)
    - Same handler as select but AdminOnly middleware checks claims.IsAdmin == true.

JWT / authentication details
- GenerateJWT(userId, isAdmin) sets a 12 hour expiration and uses the JWT_SECRET as HMAC key.
- JWTmiddleware validates Authorization header format "Bearer <token>" and ensures token is valid.
- The server stores user email/password data in a persistent "__users__" table. Sign-up writes encrypted password (bcrypt) and is_admin false by default.

Backend limitations and potential issues
- GET with JSON body is non-standard and can break clients/HTTP proxies. Prefer POST for real clients.
- Some error handling in handlers is slightly inconsistent — e.g., duplicate err checks in LoginHandler — may return incorrect error messages in some edge cases.
- No rate-limiting, CSRF protection, or login brute-force protections.
- Tokens are signed with a single secret (no key rotation implemented).
- The server assumes JWT_SECRET is correctly configured. GenerateJWT does not currently validate presence of JWT_SECRET before creating tokens; ParseWithClaims checks for unset secret during verification which returns unauthorized.
- No input schema validation or sanitization beyond value-type checks in DB layer. Clients must ensure correct shapes.

Practical curl examples

Sign-up
curl -X POST http://localhost:8080/sign-up -H "Content-Type: application/json" -d '{"email":"alice@example.com","password":"pa55"}'

Login
curl -X POST http://localhost:8080/login -H "Content-Type: application/json" -d '{"email":"alice@example.com","password":"pa55"}'

Create (insert)
curl -X POST http://localhost:8080/create -H "Content-Type: application/json" -H "Authorization: Bearer <TOKEN>" -d '{"table":"notes","values":{"title":"hello","count":1}}'

Select (get) — note: GET + body (non-standard)
curl -X GET http://localhost:8080/get -H "Content-Type: application/json" -H "Authorization: Bearer <TOKEN>" -d '{"table":"notes","where":{"field":"count","op":">","value":0}}'

Delete
curl -X DELETE http://localhost:8080/delete -H "Content-Type: application/json" -H "Authorization: Bearer <TOKEN>" -d '{"table":"notes","where":{"field":"id","op":"=","value":1}}'

---

Python client (client.py) — short usage

- The repository includes a small interactive Python client that demonstrates:
    - Sign-up and login flows (collects email/password from stdin)
    - Insert (create) with typed values
    - Select and pretty-print rows
    - Delete rows
- The client uses requests and expects server running at http://localhost:8080 by default. To run:
    - pip install requests
    - python client.py
- The script handles simple type coercion (integers and booleans) in user input. It stores the JWT token in memory for the session.

Note: the client sends GET requests with a JSON body to /get to match the backend implementation (see caveats above).

---

Development & tests

- Build & run:
  go build ./...
  ./golangdb

- Run directly:
  go run .

- Run tests:
  go test ./...

There are unit and functional tests in the repo; use go test to execute them. (Please note that tests are OUTDATED and might not work as expected or fail, as they were written for an older version of the codebase)

- Dependencies are defined in go.mod. Use go mod tidy if dependencies need to be resolved.

---

Troubleshooting & tips

- JWT_SECRET must be present in .env or as environment variable. If missing, token verification will fail.
- Database and WAL files are created under ./db/ by default. Make sure the process user can write to the working directory.
- If WAL gets corrupted (partial record), OpenDB will return an error — this typically indicates an interrupted write or file corruption. In such a case, you may:
    - Inspect WAL file (./db/wal.log) and snapshot file (./db/database.db) manually,
    - Move aside WAL file to allow server to start using only the snapshot (data since last snapshot will be lost),
    - Improve your backup/restore strategy for production (but also keep in mind that this is a educational project).
- For larger datasets you will hit memory limits: the engine keeps the entire dataset in memory. Consider sharding or using a proper external DB for large storage needs.

---

Contributing ideas & potential improvements
- Add transactional support for multi-key atomic operations.
- Add more robust snapshotting: incremental snapshots / background snapshotting to avoid long write locks.
- Add an index or secondary indices to avoid full-scan for where clauses.
- Replace GET-with-body with POST for select queries (or implement query params and pagination).
- Implement WAL rotation with compression, and more resilient WAL recovery for partial writes.
- Add rate-limiting, brute-force protection on login, and token revocation/rotation for security.

---

