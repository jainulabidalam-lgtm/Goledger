# GoLedger

A production-minded, lightweight double-entry bookkeeping application built with Go and SQLite.

## Stage 1

Currently implements a minimal HTTP server with a health check endpoint.

### Running the server

```bash
go build ./cmd/goledger
./goledger
```

Check the health endpoint: `http://localhost:8080/health`
