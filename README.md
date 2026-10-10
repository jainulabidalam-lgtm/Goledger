# GoLedger

**GoLedger** is a portfolio MVP for double-entry bookkeeping, built with Go, SQLite, and a browser-based interface embedded into the Go executable.

- **Live demo:** https://goledger-1.onrender.com
- **Source code:** https://github.com/jainulabidalam-lgtm/Goledger
- **Status:** Deployed portfolio MVP — not intended for real financial records or production accounting.

## Features

- Chart of accounts with account codes, names, types, currencies, and status.
- Double-entry journal entries with debit/credit validation and balanced-posting rules.
- Account ledger with debit, credit, and running-balance information.
- Dashboard summaries for accounts, journals, recent entries, and balances.
- Reporting currency options for INR, USD, and EUR.
- Currency converter using live exchange-rate data.
- Login and session-based authentication.
- Role-based access control for administrators, accountants, and viewers.
- CSRF protection for state-changing requests.
- Audit logging for supported accounting actions.
- Journal reversal workflow that preserves the original entry and records a reversal.
- Health endpoints and structured request logging.
- Docker-based deployment support.

## Technology stack

| Area | Technology |
|---|---|
| Backend | Go |
| Database | SQLite |
| SQLite driver | `modernc.org/sqlite` (pure Go) |
| Frontend | HTML, CSS, JavaScript |
| Frontend assets | Embedded with Go's `embed` package |
| Deployment | Docker, Render |
| Exchange rates | ExchangeRate-API open endpoint |

## Try the live demo

Open [GoLedger](https://goledger-1.onrender.com).

The hosted instance is intended for demonstration. Use fictional data only. The free Render service uses ephemeral storage and does not provide a persistent disk, so database changes may be lost when the service restarts, spins down, or is redeployed. Do not enter confidential information or rely on this instance as the only copy of any financial records.

The demo may spin down after inactivity and take a little time to respond on the next visit.

## Run locally on Windows

### Requirements

- Go version compatible with the version specified in `go.mod`.
- Git (if cloning the repository).
- A modern browser.
- Internet access for live exchange rates.

### Get the source

```powershell
git clone https://github.com/jainulabidalam-lgtm/Goledger.git
cd Goledger
```

### Run tests

```powershell
go test ./...
go vet ./...
```

### Build and run

```powershell
go build -o goledger.exe ./cmd/goledger
.\goledger.exe
```

Then open [http://127.0.0.1:8080](http://127.0.0.1:8080).

By default, the app uses `goledger.db` relative to its working directory. Run the executable from the project root unless you intentionally configure another database path.

## Configuration

GoLedger reads its runtime configuration from environment variables:

| Variable | Purpose |
|---|---|
| `HOST` | Address to bind to. Use `127.0.0.1` for local-only access and `0.0.0.0` for a container platform such as Render. |
| `PORT` | HTTP port. Render supplies this automatically; local runs default to `8080`. |
| `DB_PATH` | SQLite database path. Defaults to `goledger.db` in the working directory. |
| `PROVISION_ADMIN_USER` | Username used by the startup admin-provisioning mechanism. |
| `PROVISION_ADMIN_PASS` | Password used by the startup admin-provisioning mechanism. |

Set secrets through your deployment provider's secret/environment settings, not in source code. Confirm the application's provisioning behavior before assuming that changing provisioning variables changes an already-created account's password. Never commit database files, passwords, session tokens, or other secrets.

## Docker

The repository includes a `Dockerfile` for building a Linux Go binary and packaging the application into a small runtime image.

Build the image from the repository root:

```bash
docker build -t goledger .
```

Run it locally:

```bash
docker run --rm -p 8080:8080 -e HOST=0.0.0.0 -e PORT=8080 goledger
```

For any deployment that must preserve data, mount persistent storage and set `DB_PATH` to a file on that mounted volume. A container filesystem by itself is not durable storage.

## API overview

The application serves its embedded web interface from `/` and exposes API routes for health checks, authentication, accounts, journal entries, and account ledgers. The journal workflow also supports reversing eligible entries.

Common routes include:

| Method | Route | Purpose |
|---|---|---|
| `GET` | `/health` | Health check |
| `GET` | `/health/live` | Liveness check |
| `GET` | `/health/ready` | Readiness check |
| `POST` | `/api/auth/login` | Log in |
| `POST` | `/api/auth/logout` | Log out |
| `GET` | `/api/accounts` | List accounts |
| `GET` | `/api/journal-entries` | List journal entries |
| `GET` | `/api/journal-entries/{id}` | Retrieve a journal entry |
| `POST` | `/api/journal-entries/{id}/reverse` | Reverse an eligible journal entry |
| `GET` | `/api/accounts/{id}/ledger` | Retrieve an account ledger |

Protected endpoints require authentication. State-changing requests are subject to the application's CSRF and role checks. Refer to the route handlers in `internal/http` for exact request bodies, response formats, and authorization requirements.

## Accounting rules and data handling

- Each journal line must contain a positive debit or a positive credit, not both.
- Total debits must equal total credits before a journal entry can be posted.
- Assets and expenses normally carry debit balances; liabilities, equity, and income normally carry credit balances.
- Amounts are stored in minor currency units to avoid floating-point accounting arithmetic.
- Reporting-currency conversion is for display; it should not rewrite the original transaction amounts.

## Project structure

```text
.
├── cmd/
│   └── goledger/       # Application entry point
├── internal/
│   ├── audit/          # Audit logging
│   ├── auth/           # Authentication/session logic
│   ├── config/         # Runtime configuration
│   ├── database/       # SQLite setup and migrations
│   ├── http/           # HTTP server and API routes
│   └── ledger/         # Accounting and journal rules
├── scripts/            # Operational scripts
├── web/
│   ├── css/
│   ├── js/
│   ├── index.html
│   └── web.go           # Embedded frontend assets
├── Dockerfile
├── go.mod
└── go.sum
```

## Verification

The Go test suite and `go vet` passed during development. Run them again against the exact revision you deploy:

```powershell
go test -count=1 ./...
go vet ./...
```

Passing tests are useful evidence, but they do not replace a full security review, accounting review, or production-readiness assessment.

## Known limitations

- The hosted free-tier demo does not have durable SQLite storage.
- Exchange-rate conversion depends on a third-party service and network availability; rates can change.
- The project is a portfolio MVP, not certified accounting software.
- Before production use, add durable database storage, verified automated backups and restore tests, monitoring, and a thorough security/accounting review.

## License

No license is specified here. Add a `LICENSE` file if you want to grant others explicit permission to use, modify, or redistribute the project.
