# Mobile staff phone sign-in

`POST /v1/mobile/auth/identify` and `POST /v1/mobile/auth/otp/verify`
are public, require `X-Mobile-Api-Version: 1`, and use the usual JSON envelope.
The canonical request/response schemas are in `docs/openapi.yaml`.

Identify accepts `{ "identifier": "staff@example.com" }` and returns
`{ "kind": "email", "next": "password" }` without checking the account.
For phones, it accepts an optional ISO country `region` (default TR), normalizes
with the existing phone helper, sends an OTP and returns `kind=phone`, `next=otp`,
`phone_e164`, `resend_at` and `expires_at`. Existing and unknown numbers follow
exactly the same request path. Login limits apply to the normalized identifier;
OTP rate-limit errors preserve the portal's status, Retry-After and resend_at detail.

Verification accepts `phone`, `code`, `device`, optional `organization_slug` and
`totp_code`. Use the E.164 phone returned by identify. It issues the same mobile
JWT audience, refresh lifetime, session hydration and device-session revocation
as email/password login. Inactive accounts receive `401 INVALID_OTP_CODE`.
Customer/fleet-only accounts get a customer session and an unknown phone becomes
a customer account through the portal's account-creation rules (K11/K26); see
`customer-sessions.md`. No legacy aliases were added. Existing error codes are reused.

The shared platform OTP service owns generation, WhatsApp/SMS delivery, message
text/KVKK, limits, wrong-attempt counting and lockout. Mobile codes have the
separate `mobile_login` purpose; portal and contract codes cannot be exchanged.
Migration `000150_mobile_login_otp` only widens the existing purpose CHECK
constraint (no new table); apply it before using these endpoints.

MFA uses the same enrollment policy, TOTP/recovery verification and errors as
password login. `MFA_REQUIRED` leaves the correct OTP unconsumed. The app can
retry the **same phone/code** with `totp_code` until the original five-minute
expiry. Invalid MFA also leaves the OTP pending; every request still passes the
shared verification limiter (20 per phone+IP per 15 minutes). Wrong phone codes
retain the existing five-attempt lockout. Successful checks consume the OTP
atomically before session issuance, preventing successful replay. An error after
consumption (for example an invalid organization context) requires a new OTP.

## Local sandbox

Use the local stack with Redis, current migrations and an existing active staff
user whose `phone_e164` is, for example, `+905000001234`, and enable
`WHATSAPP_SANDBOX_PREFIXES=90500000` in the local environment. Sandbox prefixes
are rejected in production by configuration validation. The WhatsApp wrapper
swallows sends and returns a sandbox delivery reference without calling wuzapi.

**There is no fixed sandbox OTP and no `123456` bypass.** Codes are randomly
generated six-digit strings, only an HMAC is stored, and the sandbox gateway
logs metadata, not the message/code. Therefore the swallowed code cannot be read
back from the database or log. For an automated local test, the existing fake
wuzapi captures the real random code:

```sh
cd backend
GOCACHE=/tmp/gocache GOMAXPROCS=2 go test ./internal/httpserver/ \
  -run '^TestIntegrationMobilePhoneAuth$' -count=1 -v
```

This harness needs `TEST_DATABASE_URL` pointing at a migrated disposable local
database; it supplies its own in-memory Redis and mock wuzapi. It checks staff
success, customer rejection, unknown-phone noncreation, replay and version gates.

For a manual sandbox-only API test, request the OTP first:

```sh
curl -sS http://localhost:8080/v1/mobile/auth/identify \
  -H 'Content-Type: application/json' -H 'X-Mobile-Api-Version: 1' \
  -d '{"identifier":"+905000001234"}'
```

To use a known code with this swallowed delivery, replace **only the pending
local sandbox row's** `code_hash`. Get its UUID from your local database:

```sql
SELECT uuid FROM otp_codes
WHERE phone_e164 = '+905000001234' AND type = 'mobile_login'
  AND consumed_at IS NULL AND expires_at > now()
ORDER BY created_at DESC LIMIT 1;
```

With that UUID in `OTP_UUID` and the local backend's exact `APP_ENCRYPTION_KEY`
in your shell environment, compute the hash for the manual fixture code `123456`:

```sh
python3 - <<'PY'
import hashlib, hmac, os
key = hmac.new(os.environ['APP_ENCRYPTION_KEY'].encode(),
               b'otp-code-hash-v1', hashlib.sha256).digest()
print(hmac.new(key, (os.environ['OTP_UUID'] + ':123456').encode(),
               hashlib.sha256).hexdigest())
PY
```

Paste the UUID/hash into this query in that same **local** database:

```sql
UPDATE otp_codes SET code_hash = '<computed hash>'
WHERE uuid = '<OTP_UUID>' AND phone_e164 = '+905000001234'
  AND type = 'mobile_login' AND consumed_at IS NULL AND expires_at > now();
```

Then verify before the original expiry (include `totp_code` if enabled):

```sh
curl -sS http://localhost:8080/v1/mobile/auth/otp/verify \
  -H 'Content-Type: application/json' -H 'X-Mobile-Api-Version: 1' \
  -d '{"phone":"+905000001234","code":"123456","device":{"id":"local-phone","platform":"android"}}'
```

The known code exists only because of the local fixture update; neither the
application nor sandbox configuration accepts a fixed code automatically.
