# Design — Publication de l'image conteneur sur GHCR

Date : 2026-06-20
Repo : `samouraiworld/gno-onboarding-bot`

## Objectif

Construire et publier automatiquement l'image Docker du bot sur GitHub
Container Registry (GHCR), faire consommer cette image par `docker-compose.yml`
au lieu d'un build local, et garantir qu'aucun secret n'est embarqué dans
l'image.

## Périmètre

Trois changements, aucun code applicatif touché :

1. Nouveau workflow GitHub Actions `.github/workflows/docker-publish.yml`.
2. `docker-compose.yml` : passer de `build: .` à `image: ghcr.io/...`.
3. Renforcement défensif du `.dockerignore` + documentation de la garantie
   « pas de secret dans l'image ».

## 1. Workflow GitHub Actions

Fichier : `.github/workflows/docker-publish.yml`

**Déclencheurs :**
- `push` sur la branche `master` → tags `latest` et `sha-<court>`.
- `push` d'un tag `v*` → tags semver (`vX.Y.Z`, `X.Y`) via `docker/metadata-action`.

**Permissions du job (minimum) :**
```yaml
permissions:
  contents: read
  packages: write
```

**Étapes :**
1. `actions/checkout@v4`
2. `docker/setup-buildx-action@v3`
3. `docker/login-action@v3` → registry `ghcr.io`, `username: ${{ github.actor }}`,
   `password: ${{ secrets.GITHUB_TOKEN }}` (token intégré, aucun secret à créer).
4. `docker/metadata-action@v5` → image `ghcr.io/samouraiworld/gno-onboarding-bot`,
   calcule tags (`latest` sur master, semver sur tags, `sha-`) et labels OCI.
5. `docker/build-push-action@v6` → `push: true`, tags/labels issus de l'étape 4,
   cache GHA (`cache-from`/`cache-to: type=gha`).

Le nom d'image GHCR doit être en minuscules : `ghcr.io/samouraiworld/gno-onboarding-bot`.

## 2. docker-compose.yml

Remplacer :
```yaml
    build: .
```
par :
```yaml
    image: ghcr.io/samouraiworld/gno-onboarding-bot:latest
```
Inchangé : `container_name`, `restart: unless-stopped`, et les trois volumes
en lecture seule montant `config.yaml`, `service-account.json`, `templates.yaml`
au runtime.

## 3. Garantie « aucun secret dans l'image »

Déjà vrai par construction, à documenter et renforcer :

- Le `Dockerfile` est multi-stage : le stage final (`alpine:3.20`) ne copie que
  le binaire compilé (`COPY --from=build /out/onboardingbot`). Le `COPY . .` du
  stage de build est jeté et n'atteint jamais l'image finale.
- Le `.dockerignore` exclut déjà `config.yaml`, `service-account.json`, `*.env`,
  `.git` du contexte de build.
- `config.yaml` et `service-account.json` sont gitignorés → absents du runner CI.
- Le workflow n'utilise aucun `--build-arg` ni `secrets:` de build.

**Renforcement défensif du `.dockerignore`** — ajouter :
```
*.local.yaml
*.pem
*.key
```

## Hors-scope (YAGNI)

- Build multi-architecture (arm64).
- Scan de vulnérabilités (Trivy) et signature d'image (cosign).

## Critères de succès

- Un push sur `master` produit `ghcr.io/samouraiworld/gno-onboarding-bot:latest`.
- Un tag `vX.Y.Z` produit l'image taguée en conséquence.
- `docker compose pull && docker compose up -d` démarre le bot avec les secrets
  montés en volume.
- L'image finale ne contient que le binaire (vérifiable : pas de `config.yaml`
  ni `service-account.json` dedans).
