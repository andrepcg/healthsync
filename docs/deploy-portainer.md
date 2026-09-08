# Deploying healthsync on Portainer

healthsync ships as a single container: the Go server with the dashboard UI
embedded. All state lives in one volume (`/data`), which holds `people.db` and
one SQLite database per family member under `people/`.

> **No authentication.** The dashboard is meant for a home LAN (or a Tailscale
> network). Do not port-forward it to the internet: it holds medical data for
> everyone in the family.

## 1. Image

The GitHub Actions workflow `.github/workflows/docker.yml` publishes a
multi-arch image (amd64 + arm64) to GHCR on every push to `main` and on `v*`
tags:

```
ghcr.io/<your-github-user>/healthsync:latest
```

One-time step after the first push: on GitHub, open the package
(`Packages → healthsync → Package settings`) and set its visibility to
**Public** so Portainer can pull it without credentials. If you prefer to keep
it private, add a registry in Portainer (`Registries → Add registry → Custom`,
URL `ghcr.io`, your username and a token with `read:packages`).

## 2. Stack

In Portainer: **Stacks → Add stack**.

**Option A – Repository (recommended, GitOps-friendly)**

- Repository URL: your fork, e.g. `https://github.com/<you>/healthsync`
- Reference: `refs/heads/main`
- Compose path: `docker-compose.yml`
- Optionally enable *GitOps updates* so a push to `main` redeploys.

**Option B – Web editor**

Paste the contents of `docker-compose.yml` and edit the `image:` line to your
GHCR path.

Environment variables you may want to change (Portainer: the **Environment
variables** section of the stack; docker compose: a `.env` file, see
`.env.example`):

| Variable | Default | Purpose |
|---|---|---|
| `HEALTHSYNC_PORT` | `8080` | **Host** port the dashboard is published on. Set this if 8080 is taken, e.g. `HEALTHSYNC_PORT=8090`. The container always listens on 8080 internally. |
| `TZ` | `Europe/Lisbon` | Container timezone (log timestamps). Health data itself is stored in the wearer's local wall-clock time as exported. |
| `PORT` | `8080` | Port the process listens on *inside* the container. Only change this with `network_mode: host`; with a normal port mapping leave it alone. |
| `HEALTHSYNC_DATA_DIR` | `/data` | Where databases are written. Keep it on the volume. |

Deploy, then open `http://<host>:<HEALTHSYNC_PORT>`.

## 3. First run

1. The app opens on **People & imports**. Add a person.
2. On the iPhone: **Health → profile picture → Export All Health Data**, then
   AirDrop / share `export.zip` to a computer.
3. Drop `export.zip` on the person's card. A full export (hundreds of
   thousands of records) imports in well under a minute; progress is shown live.
4. Repeat for each family member. Re-uploading a newer export is safe: existing
   rows are skipped, new ones added.

Date of birth and sex are filled in automatically from the export if you left
them blank.

## 4. Bind mounts and permissions

The container runs as uid/gid 1000. The named volume in the compose file
inherits the right ownership automatically. If you switch to a bind mount, make
the host directory writable by uid 1000:

```sh
sudo mkdir -p /srv/healthsync && sudo chown 1000:1000 /srv/healthsync
```

## 5. Backups

Everything is in the `healthsync-data` volume. Either back up the volume, or
download a consistent snapshot of one person's database from the UI
(**Explore data → Download SQLite**, or `GET /api/people/<id>/export.db`).

## 6. Updating

Portainer: **Stacks → healthsync → Pull and redeploy**. Schema changes are
applied automatically when the new version opens each database.

## 7. Building locally instead of GHCR

```sh
make docker            # docker build -t healthsync:local .
docker compose up -d   # after switching `image:` to `build: .`
```
