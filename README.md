# cellmate

A social and dating platform for criminals.

## Run

```sh
go run ./cmd/cellmate
```

Listens on `:8080`. Data goes to `data/cellmate.json`, which is created if missing. Requires Go 1.24 or newer.

```sh
PORT=3000 DATA_FILE=/var/lib/cellmate/cellmate.json go run ./cmd/cellmate
```

## Env vars

- `PORT` listen port. Default `8080`.
- `DATA_FILE` path to the JSON database. Default `data/cellmate.json`. Photos go in a `photos/` directory next to it. `data/` is gitignored.

## Routes

- `GET /` landing page with Log in (`/login`) and Create account (`/signup`).
- `GET /signup` signup form with email, password, and the required age and legal consent checkbox.
- `POST /signup` creates the account and a session, then redirects to `/home`, which sends a new user on to `/profile`. Returns 400 for a bad email, a password under 8 characters, or no consent. Returns 409 for an email that is already registered.
- `GET /login` login form.
- `POST /login` creates a session and redirects to `/home`. Wrong email or wrong password both return 401 with the same "Wrong email or password." message.
- `GET /home` shows today's lineup (or today's pick), the Matches list (photo and name), the user's own profile card with an Edit profile link, "Logged in as <email>", and a Log out button. Redirects to `/profile` when the display name is empty. Redirects to `/login` without a valid session.
- `POST /lineup/pick` with form field `member` picks that member from today's lineup and redirects to `/home`. Returns 400 when the member is not in today's lineup and 409 when the user already picked today, both with the message on the `/home` page. Redirects to `/login` without a valid session.
- `GET /profile` edit form for the logged-in user's own profile, filled with the current values. Redirects to `/login` without a valid session.
- `POST /profile` multipart form with `display_name`, `bio`, `convictions`, and an optional `photo` file. Saves only the session user's profile and redirects to `/home`. Any user id in the form is ignored. Bad input re-shows the form with the submitted values and a message: 400 for a missing or long field or a photo that is not JPEG or PNG, 413 for a photo over 5 MB. Leaving the file empty keeps the current photo.
- `GET /photos/{id}` serves a photo to any logged-in user. Returns 401 when logged out and 404 for an unknown or malformed id.
- `POST /logout` deletes the session, clears the cookie, and redirects to `/`.
- `GET /static/style.css` the embedded stylesheet.
- `GET /`, `GET /login`, and `GET /signup` redirect to `/home` when already logged in.
- Other methods on these routes return 405.

## The Lineup

- Each day a user gets up to 5 other members, picked at random, shown as mug shots against a height chart. A member with no photo shows a silhouette. Clicking a member opens their bio and conviction history with a Pick button.
- Candidates are users with a display name, excluding the user, everyone the user has ever picked, and the user's matches. Fewer than 5 candidates shows all of them. None shows an empty state.
- The lineup is generated on the first `/home` load of the day and saved, so a refresh shows the same members. Days roll over at midnight `America/Los_Angeles`, so the binary needs the system time zone database.
- A user picks at most one member per day. After the pick, `/home` shows the picked member and says the next lineup comes at midnight Pacific.
- A match forms when A has picked B and B has picked A, on any days. Each pair is stored once.

## Sessions

- Cookie `session` holds 32 random bytes, base64url. It is `HttpOnly`, `SameSite=Lax`, `Path=/`, and `Max-Age` 30 days.
- The cookie is `Secure` when the request came over TLS or carries `X-Forwarded-Proto: https` from the ingress.
- The data file stores only the SHA-256 of each token, with its user id and expiry. Sessions survive a restart. Expired sessions are rejected and pruned on the next login.

## Data file

- One JSON file with `users`, `sessions`, `lineups`, `picks`, and `matches`. A mutex guards it. Every write goes to a temp file in the same directory, is fsynced, then renamed over the old file.
- A user has `id`, `email` (trimmed, lowercased, unique), `password_hash`, `created_at`, `consent_at`, and `profile`.
- `profile` has `display_name` (required, 1 to 40 characters), `photo_id`, `bio` (up to 500 characters), and `convictions` (conviction history, up to 500 characters). Text is trimmed and lengths count Unicode characters. An empty `display_name` means the profile is not set up yet. In Go this is `User.Profile`, saved with `Store.UpdateProfile`.
- Photos are stored as `photos/<photo_id>`, where the id is 32 random hex characters, never the uploaded filename. The type is checked by sniffing the first bytes with `http.DetectContentType`, not by the extension or the client's content type. Uploads are decoded and re-encoded (JPEG quality 90, PNG default) so EXIF and GPS metadata are dropped, and images over 8000 by 8000 pixels are rejected as not a valid photo. Saving a new photo deletes the old file.
- `lineups` maps a user id to the latest lineup: `day` (`YYYY-MM-DD` in Pacific time), `member_ids`, and `picked_id`. `picks` maps a user id to every member id they have picked. `matches` holds `users` (the two ids, sorted) and `created_at`.
- Passwords are hashed with PBKDF2-HMAC-SHA256 from `crypto/pbkdf2`, 600,000 iterations, 16-byte random salt, stored as `pbkdf2-sha256$<iterations>$<salt>$<key>`. Checks compare in constant time.

Not yet handled: messaging, unmatching, blocking, removing a photo without replacing it, image resizing, password reset, email verification, rate limiting, CSRF tokens beyond `SameSite=Lax`.

## Layout

- `cmd/cellmate/main.go` server, routes, and handlers.
- `cmd/cellmate/store.go` JSON file store for users, profiles, and sessions.
- `cmd/cellmate/profile.go` profile form handlers and validation.
- `cmd/cellmate/lineup.go` daily lineup, picks, matches, and the `/lineup/pick` route.
- `cmd/cellmate/photo.go` photo files on disk and the `/photos/{id}` route.
- `cmd/cellmate/password.go` password hashing.
- `cmd/cellmate/web/` embedded templates and CSS.

## Test

```sh
go test ./...
```
