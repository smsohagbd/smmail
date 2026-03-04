# Go SMTP Platform (Postfix-like control plane)

Features implemented:
- SMTP AUTH (PLAIN and LOGIN)
- Per-user rate limits: per-second, per-minute, per-hour, per-day
- Per-user throttling (minimum milliseconds between sends)
- Per-domain throttling and per-domain hourly limit per user
- Outbound queue + delivery worker + retry/fail logic
- Admin API to create/manage users and limits
- Admin dashboard for account, throttle, and event operations

## Run

1. Install Go 1.22+
2. Set environment values in `.env`:
   - `ADMIN_USERNAME`
   - `ADMIN_PASSWORD`
   - `UPSTREAM_SMTP_*` (for real outbound relay)
3. Fetch deps:
   - `go mod tidy`
4. Start server:
   - `go run ./cmd/smtpd`

## Dashboards

- Single Login (admin/user redirect): `http://localhost:8080/ui/index.html`
- Admin: `http://localhost:8080/ui/admin.html`
- User: `http://localhost:8080/ui/user.html`
- SMTP tester: `http://localhost:8080/ui/smtp-tester.html`

### Admin login
- Login happens only on `/ui/index.html` and redirects by role.
- API login endpoint: `POST /api/admin/login`
- Admin API requests use `X-Admin-Session` header internally from UI.

### Admin features
- Global overview cards: users, new users (24h), total logs, 24h sent/failed, queue states.
- Package manager: create/update plans and set default auto-assigned package.
- SMTP pool manager: add many upstream SMTP accounts and assign them per user.
- Account plan management: `plan_name`, `monthly_limit`, user limits/throttles, rotation switch.
- Admin can manually assign package to any user.
- Full delete operations: delete users, packages (non-default), SMTPs, and SMTP assignments.
- Per-account usage table and recent send logs.
- Sending log pagination.

### User features
- User registration endpoint with auto default-package assignment.
- User login with SMTP credentials.
- User can add own upstream SMTP accounts.
- User can assign multiple SMTP accounts and enable rotation.
- User can view package catalog and change plan from dashboard.
- User can unassign and delete their own SMTP accounts.
- Personal usage: month/day sent and failed.
- Personal sending log history with pagination.

## Local SMTP test with username/password

1. Edit `smtp-test.local.json` and set:
   - `smtp_username`, `smtp_password`
   - `from`, `to`
2. Run:
   - `powershell -ExecutionPolicy Bypass -File .\scripts\send-test.ps1`
3. If your config file has another name/path:
   - `powershell -ExecutionPolicy Bypass -File .\scripts\send-test.ps1 -ConfigPath .\my-config.json`

If auth fails, first create the SMTP user in admin dashboard/API, then retry.

## Important
This is a strong base, not a complete replacement for Postfix. For production internet sending, add:
- DKIM signing
- SPF/DMARC alignment and bounce processing
- Queue sharding + persistent distributed rate-limiter
- TLS cert management and forced TLS policies
- Abuse detection and IP/domain reputation controls

## Delivery behavior
- App no longer uses `.env` upstream SMTP fallback for outbound delivery.
- Outbound sending requires assigned/enabled upstream SMTP(s) for the user.
- Message envelope/header sender address is forced to selected upstream SMTP `from_email`.


git init
git remote add origin https://github.com/your-username/ecom-project.git
git add .
git commit -m "first commit"
git branch -M main
git push -u origin main