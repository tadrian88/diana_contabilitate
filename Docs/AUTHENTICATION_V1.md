# Diana Authentication V1

## Boundary and flow

Authentication V1 is application authentication, separate from both Google Cloud perimeter controls and ANAF OAuth.

```text
Browser -> /login -> POST /api/v1/auth/login
        -> normalized email + Argon2id verification
        -> opaque random session and CSRF secrets
        -> PostgreSQL auth_sessions (hashes only)
        -> host-only Secure/HttpOnly diana_session cookie
        -> session lookup on every protected API request
        -> auth_users + auth_user_client_grants
        -> RequestActor
        -> existing client authorization checks
```

The frontend has one authoritative `AuthProvider`. Startup calls `GET /api/v1/auth/session` before rendering the protected shell. A 401 clears frontend auth state and routes to `/login`. No bearer or refresh token is stored in browser storage.

## ARMQU discovery and adaptation

ARMQU's inspected flow is `/login` -> `LoginForm` -> `POST /v1/auth/login` -> Ent `User` lookup -> bcrypt verification -> access/refresh JWTs -> browser `localStorage` -> bearer interceptor -> Redis-backed session blacklist on logout. Its split-screen `AuthLayout`, centered `max-w-md` form, responsive mobile logo, labeled 44px inputs, password visibility toggle, inline generic error, disabled/loading submit, guest/protected routes, identity in the shell, and explicit logout are the applicable product patterns.

No Google sign-in entry or Google authentication provider was found in the inspected ARMQU login/router/service flow. ARMQU's Google-related dependencies are not the cause of Diana's redirect. Diana also intentionally omits ARMQU signup, forgot-password, TOTP, API-key and full tenant/license RBAC flows from V1.

| ARMQU concept | ARMQU implementation | Diana equivalent | Action |
|---|---|---|---|
| Login layout | Split image/brand panel and centered form | Diana design tokens and brand panel | ADAPT |
| Form behavior | React Hook Form, visibility toggle, spinner, generic error | Controlled Diana form with the same behavior | ADAPT |
| Password verification | bcrypt | Argon2id PHC hashes | ADAPT |
| Session | short JWT + refresh JWT in localStorage; Redis blacklist | opaque PostgreSQL session; HttpOnly cookie | REJECT JWT storage / ADAPT lifecycle |
| Route guard | token presence plus cached user | server session bootstrap plus protected shell | ADAPT |
| 401 handling | refresh, then login | clear auth state and login | ADAPT |
| User status | `is_active` | `ACTIVE` / `DISABLED` | REUSE concept |
| RBAC | tenant role and licenses | existing RequestActor/client grants; one CONTABIL persona | REJECT ARMQU RBAC |
| Provisioning | signup/admin flows | operator-only CLI | REJECT self-registration / ADAPT admin boundary |
| Google authentication | not the Diana perimeter model | existing GCP IAP only | REJECT as primary TEST entry |

## Data model

- `auth_users`: normalized unique email, Argon2id hash, ACTIVE/DISABLED status, CONTABIL persona, optional all-clients authority and credential version.
- `auth_user_client_grants`: explicit user-to-AccountingClient grants.
- `auth_sessions`: random session token hash, CSRF token hash, expiry, revocation and activity timestamps. Plain session secrets are never persisted.
- `auth_events`: bounded security event types without password or session material.

`all_clients=true` is explicit authority for an accountant who must onboard and work across all TEST clients. It is not inferred from an email, header, query parameter or frontend state.

## Password security

Passwords use Argon2id with a random 16-byte salt, 64 MiB memory, three iterations, parallelism two and a 32-byte result. The PHC string carries its parameters. Login performs a dummy Argon2id verification when an email is unknown and always returns the same invalid-credential response. Email normalization is `lower(trim(email))`; no provider-specific alias rules exist.

The operator CLI reads a password without terminal echo or accepts it through protected stdin/secret injection. Provisioning refuses duplicates. Password reset is a separate explicit operation and revokes existing sessions.

Local provisioning after migration:

```bash
cd backend
DATABASE_URL='postgresql://diana:diana@127.0.0.1:5442/diana?sslmode=disable' \
  go run ./cmd/authuser provision --email demo@accountingtechco.com --all-clients
```

The command prompts twice without echo. Reset uses the same command with
`reset-password` in place of `provision`.

## Session, cookie and CSRF model

Sessions are authoritative in PostgreSQL, survive API restarts/revisions and work across Cloud Run instances. Default lifetime is 12 hours. Disabled users fail every subsequent session validation.

`diana_session` is host-only, `HttpOnly`, `SameSite=Lax`, `Path=/`, and `Secure` in cloud-test/production. `diana_csrf` is a host-only readable cookie with the same scope and lifetime; only its SHA-256 hash is stored. Unsafe protected requests must present the matching `X-CSRF-Token`, and login/logout validate the configured frontend Origin when a browser supplies it. SameSite is defense in depth, not the sole CSRF control. No broad `.accountingtechco.com` Domain is used.

The live load-balancer URL map routes `test.platform.accountingtechco.com/api/*` to the API, so frontend and API are same-origin. The ANAF callback remains the exact public route on `api-test.accountingtechco.com`.

## Public and protected routes

Public application/protocol routes:

- `POST /api/v1/auth/login`
- `GET /api/v1/integrations/anaf/callback`
- `/healthz` and `/readyz` for platform health

`GET /api/v1/auth/session` and `POST /api/v1/auth/logout` require a session. Every other `/api/*` route and `/metrics` requires a valid Diana session; unsafe methods additionally require CSRF proof. This includes clients/settings, invoices, contracts, rules, tasks, ANAF client configuration, contract source files and SAGA artifacts.

## ANAF compatibility

Diana login and ANAF qualified-certificate OAuth remain distinct. OAuth start is protected and bound to a client through durable, random, single-use state. The provider callback must stay anonymously reachable because the provider cannot authenticate as a Diana user. It consumes the stored state before token exchange, checks environment and replay, and redirects to the configured Diana frontend. The callback does not need the host-only Diana cookie.

## Rate limiting and failure behavior

Login attempts are counted in shared Redis for 15 minutes (default five) using a SHA-256-derived key over normalized email and source IP. Redis failure fails login closed. Invalid password, unknown user and disabled user share the safe response. Passwords are excluded from logs, audit rows and errors.

## Deliberate scope

There is no signup, invitation, forgot-password email, MFA, SSO, SAML, SCIM, social login or public user administration endpoint. Session pruning and richer production RBAC remain later operational work.
