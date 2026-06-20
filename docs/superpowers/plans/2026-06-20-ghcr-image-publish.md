# GHCR Image Publishing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and publish the bot's Docker image to GitHub Container Registry on every push to `master` and on `v*` tags, and make `docker-compose.yml` consume that image instead of building locally.

**Architecture:** A GitHub Actions workflow logs into `ghcr.io` with the built-in `GITHUB_TOKEN`, computes tags via `docker/metadata-action`, and builds+pushes via `docker/build-push-action`. The existing multi-stage `Dockerfile` already guarantees the final image contains only the compiled binary, so no secret can be embedded. `docker-compose.yml` switches to pulling the published image.

**Tech Stack:** GitHub Actions, Docker Buildx, GitHub Container Registry (GHCR), existing Go multi-stage Dockerfile.

## Global Constraints

- Image name (lowercase, GHCR): `ghcr.io/samouraiworld/gno-onboarding-bot`
- `latest` tag rule: `type=raw,value=latest,enable={{is_default_branch}}`
- Job permissions limited to `contents: read` and `packages: write`
- No build secrets: workflow must use no `--build-arg` and no build `secrets:`
- Never commit `config.yaml` or `service-account.json` (already gitignored)

---

### Task 1: Harden .dockerignore

**Files:**
- Modify: `.dockerignore`

**Interfaces:**
- Consumes: nothing
- Produces: a build context that excludes any stray credential files

- [ ] **Step 1: Update `.dockerignore`**

Replace the full contents of `.dockerignore` with:

```
.git
config.yaml
service-account.json
*.env
*.local.yaml
*.pem
*.key
```

- [ ] **Step 2: Verify the build context excludes secrets**

Run:
```bash
docker build -t ghcr-test . && \
docker run --rm --entrypoint sh ghcr-test -c 'ls -la /app'
```
Expected: `/app` lists only `onboardingbot` (no `config.yaml`, no `service-account.json`, no `templates.yaml`). The multi-stage final image copies only the binary.

- [ ] **Step 3: Commit**

```bash
git add .dockerignore
git commit -m "Harden .dockerignore against stray credential files"
```

---

### Task 2: Point docker-compose.yml at the GHCR image

**Files:**
- Modify: `docker-compose.yml:3`

**Interfaces:**
- Consumes: the image name from Global Constraints
- Produces: a compose file that pulls instead of builds

- [ ] **Step 1: Replace the build directive with the image reference**

In `docker-compose.yml`, change line 3 from:
```yaml
    build: .
```
to:
```yaml
    image: ghcr.io/samouraiworld/gno-onboarding-bot:latest
```

The resulting file is:
```yaml
services:
  bot:
    image: ghcr.io/samouraiworld/gno-onboarding-bot:latest
    container_name: gno-onboarding-bot
    restart: unless-stopped
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - ./service-account.json:/app/service-account.json:ro
      - ./templates.yaml:/app/templates.yaml:ro
```

- [ ] **Step 2: Validate the compose file**

Run: `docker compose config`
Expected: prints the resolved config with `image: ghcr.io/samouraiworld/gno-onboarding-bot:latest` and no `build:` key, exit code 0.

- [ ] **Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "Pull bot image from GHCR instead of building locally"
```

---

### Task 3: Add the GHCR publish workflow

**Files:**
- Create: `.github/workflows/docker-publish.yml`

**Interfaces:**
- Consumes: image name, `latest` rule, and permissions from Global Constraints
- Produces: published images at `ghcr.io/samouraiworld/gno-onboarding-bot`

- [ ] **Step 1: Create the workflow file**

Create `.github/workflows/docker-publish.yml` with:

```yaml
name: Publish Docker image to GHCR

on:
  push:
    branches:
      - master
    tags:
      - 'v*'

env:
  IMAGE_NAME: ghcr.io/samouraiworld/gno-onboarding-bot

jobs:
  build-and-push:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata (tags, labels)
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.IMAGE_NAME }}
          tags: |
            type=raw,value=latest,enable={{is_default_branch}}
            type=sha,prefix=sha-
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

- [ ] **Step 2: Lint the workflow YAML locally**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/docker-publish.yml')); print('ok')"`
Expected: prints `ok`, exit code 0.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/docker-publish.yml
git commit -m "Add GitHub Actions workflow to publish image to GHCR"
```

- [ ] **Step 4: Post-merge verification (manual, after pushing to master)**

After this lands on `master` and is pushed:
1. Open the repo's **Actions** tab → confirm the "Publish Docker image to GHCR" run is green.
2. Open the repo's **Packages** → confirm `gno-onboarding-bot` exists with a `latest` tag and a `sha-<short>` tag.
3. On the deploy host: `docker compose pull && docker compose up -d` → bot starts with secrets mounted via volumes.

Note: GHCR packages are private by default. If the deploy host pulls anonymously, set the package visibility to public, or run `docker login ghcr.io` with a PAT (`read:packages`) on that host.

---

## Notes for the implementer

- The `Dockerfile` is unchanged. The "no secrets in image" guarantee comes from its multi-stage design (final stage copies only `/out/onboardingbot`) plus `.dockerignore`. Task 1 Step 2 verifies this empirically.
- `docker/metadata-action`'s `enable={{is_default_branch}}` resolves to true only on `master` pushes, so tag pushes won't clobber `latest` unintentionally (they get semver tags instead).
- No repository secrets need to be created: `GITHUB_TOKEN` is provided automatically by Actions and the `packages: write` permission authorizes the push.
