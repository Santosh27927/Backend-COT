# Local troubleshooting

| Symptom | Check |
|---|---|
| `go` missing or module version rejected | Install Go 1.27.4 and reopen the terminal if PATH needs refreshing |
| Docker connection error | Start Docker Desktop before `make db-up` |
| Missing `.env` | Run `make init-local`; it preserves existing files |
| Token configuration rejected | Use a generated token; the example placeholders are deliberately invalid |
| Port 55432 already occupied | Choose another COT_DB_PORT and update COT_DATABASE_URL accordingly; do not stop unrelated services |
| Readiness says database_unavailable | Check the COT container, URL, credentials, and port |
| Readiness says migrations_required | Run `make migrate` and inspect its exit code |
| Readiness says migration_mismatch | Restore the original applied migration; do not erase history to hide drift |
| `/v1/me` returns 401 | Use the generated local token; username headers do not authenticate |
| LAN binding rejected | Set COT_HTTP_ADDR and COT_ALLOW_LAN=true explicitly on a trusted testing network |
| Phone cannot reach localhost | Use the development computer's reachable LAN address; the phone's localhost is the phone |

Correlate failures with X-Request-ID and the JSON service logs. Never paste the
private `.env`, bearer token, or database URL into an issue or public log.

Ctrl-C or SIGTERM stops each Go service gracefully. `make db-stop` stops only
the COT PostgreSQL container and preserves its volume.

The local database role can create databases for integration tests. Production
roles and public authentication are not part of this development setup.
