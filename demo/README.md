# Public Demo

This directory holds everything that turns a Shisho release into the read-only Public Demo at `https://demo.shishobooks.com`. It contains no application code.

| File                            | Purpose                                                                                                           |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `Dockerfile`                    | Derived image: `FROM ghcr.io/shishobooks/shisho:<release>` plus the Demo Corpus copied in, with `DEMO_MODE=true`. |
| `fly.toml`                      | Fly.io app config: one `shared-cpu-1x` 512 MB Machine that suspends when idle.                                    |
| `docker-compose.yml`            | Authoring setup: the pinned release image without Demo Mode, mounted on a local corpus clone.                     |
| `../.github/workflows/demo.yml` | Deploys on every published release, on manual dispatch, and when the corpus repository pushes to `master`.        |

The media, covers, sidecars, prepared database, and credits live in the separate data repository [`shishobooks/demo-corpus`](https://github.com/shishobooks/demo-corpus). Media never enters this repository. The vocabulary (Demo Mode, Public Demo, Demo Corpus) is defined in `CONTEXT.md`, and the full specification is issue #473.

## How a deploy works

1. `demo.yml` checks out this repository and `shishobooks/demo-corpus@master` into `demo/corpus` (gitignored).
2. It resolves the release to deploy: the published release's tag, the `version` input, or the latest release.
3. It waits for `ghcr.io/shishobooks/shisho:<version>` to exist. The Release workflow publishes the GitHub release and the Docker manifest from separate jobs, so on `release: published` the image can lag by a few minutes. Note that the multi-arch manifest tag carries no `v` prefix (`0.0.51`, not `v0.0.51`).
4. `flyctl deploy` builds `demo/Dockerfile` on Fly's remote builder with the corpus in the build context and rolls the single Machine. Nothing is downloaded at Machine start; the database and media are already in the image.

Demo Mode itself (write-rejecting middleware, skipped workers and plugins, hidden downloads) is application behavior. See the Demo Mode section of `pkg/CLAUDE.md` and `website/docs/configuration.md`.

## Authoring and updating the corpus

The database is authored with Shisho itself. Nothing scans at demo startup, so whatever you commit is exactly what visitors see.

1. Clone the corpus next to this repository: `git clone https://github.com/shishobooks/demo-corpus.git ../demo-corpus`. If it lives elsewhere, export `DEMO_CORPUS_DIR` (relative to `demo/`) before the next step.
2. Start the authoring instance: `docker compose -f demo/docker-compose.yml up`. This runs the release pinned in the Compose file without Demo Mode, with the corpus `library/` mounted at `/media` and its `config/` at `/config`. Open `http://localhost:5173`.
3. First time only (the corpus already ships a prepared database, so this only applies to a rebuild from scratch):
   - Complete setup with the throwaway admin credential recorded in the corpus `CORPUS.md`. The database is public, so never use a real password.
   - Create one library at `/media` with file organization enabled and let the scan finish.
   - Create user `demo` with the Viewer role and password `shishodemo`, without requiring a password reset. Sign in as `demo` once to confirm the login lands on the library rather than a forced password change.
4. Curate metadata, series, genres, tags, and descriptions in the UI. Every Book description must end with a one-line credit and license so attribution is visible without leaving the app. To add or replace works, put the files under `library/` (one directory per Book), rescan, and add the work to `CORPUS.md` and `build-corpus.sh`.
5. Stop the container. Commit and push the corpus repository. `config/cache/` is gitignored; commit `config/shisho.db`, `library/`, covers, and sidecars. The push to `master` sends a `repository_dispatch` to this repository, which redeploys the demo.

When a new Shisho release changes the schema, bump `SHISHO_VERSION` in `docker-compose.yml` and repeat the loop; the demo runs pending migrations at startup, but committing an already-migrated database keeps Machine cold starts short.

Do not add supplements to the corpus. The generated download route serves supplements as originals, and only works that are freely redistributable belong in the demo.

## Redeploying manually

- From GitHub: run the **Demo** workflow (`gh workflow run demo.yml`, optionally `-f version=v0.0.51`). Without a version it deploys the latest release with the current corpus.
- From a machine with `flyctl` signed in:

  ```sh
  git clone https://github.com/shishobooks/demo-corpus.git demo/corpus
  cd demo
  flyctl deploy --config fly.toml --dockerfile Dockerfile --build-arg SHISHO_VERSION=0.0.51 --remote-only --ha=false
  ```

  `--ha=false` keeps the app at one Machine. Fly otherwise creates two on a fresh app.

## Rotating `JWT_SECRET`

The secret only signs visitor sessions; rotating it signs everyone out and nothing else.

```sh
fly secrets set JWT_SECRET="$(openssl rand -hex 32)" --app shisho-demo
```

Setting a secret restarts the Machine, so no redeploy is needed.

## One-time operator setup

These steps need a Fly.io account, DNS access for `shishobooks.com`, and admin rights on both GitHub repositories. Each is idempotent enough to re-run.

1. Sign in and create the app in your Fly organization:

   ```sh
   fly auth login
   fly apps create shisho-demo
   ```

2. Set the session-signing secret:

   ```sh
   fly secrets set JWT_SECRET="$(openssl rand -hex 32)" --app shisho-demo
   ```

3. Create an app-scoped deploy token and store it as `FLY_API_TOKEN` in this repository's Actions secrets:

   ```sh
   fly tokens create deploy --app shisho-demo --name "github-actions demo deploy" --expiry 8760h
   gh secret set FLY_API_TOKEN --repo shishobooks/shisho
   ```

4. Create a fine-grained GitHub token so the corpus repository can trigger deploys here. On <https://github.com/settings/personal-access-tokens/new>, grant it access to only `shishobooks/shisho` with **Contents: read and write** (the permission `repository_dispatch` requires). Store it in the corpus repository:

   ```sh
   gh secret set SHISHO_DISPATCH_TOKEN --repo shishobooks/demo-corpus
   ```

5. Run the first deploy (`gh workflow run demo.yml`) so the app has a Machine, then point DNS at it: a `CNAME` record for `demo` to `shisho-demo.fly.dev`.

6. Issue the certificate and wait for it to validate:

   ```sh
   fly certs add demo.shishobooks.com --app shisho-demo
   fly certs check demo.shishobooks.com --app shisho-demo
   ```

The demo needs a release that includes Demo Mode. Cut one with `mise release <version>` from `master`; the Release workflow publishes the image and the Demo workflow deploys it.

## Cost and safety notes

- A suspended Machine is billed as storage only. Fly has no billing alerts, so glance at the bill monthly. If cold starts are annoying, set `min_machines_running = 1` in `fly.toml`.
- The concurrency limits in `fly.toml` are the only rate limiting. Demo Mode rejects every persistent change before any handler runs, so there is nothing to reset between visitors.
- The admin account in the prepared database is a throwaway. Admins have no bypass in Demo Mode.
