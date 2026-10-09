#!/usr/bin/env bash
# Deploys the bot on the lab. Run it from the directory holding compose.yml
# (copied from compose.example.yml), .env and data/.
#
#   ./deploy.sh                 pull :latest from ghcr.io
#   ./deploy.sh v1.2.3          pull :v1.2.3 from ghcr.io
#   ./deploy.sh bot.tar.gz      docker load a Release tarball (no registry needed)
#
# Then it runs `docker compose up -d`. A private ghcr.io package needs
# `docker login ghcr.io` first.
set -euo pipefail

BOT_IMAGE="${BOT_IMAGE:-ghcr.io/aligh5331/personal-budget-manger}"
arg="${1:-latest}"

if [[ $# -gt 1 ]]; then
  echo "usage: $0 [vX.Y.Z | latest | image.tar.gz]" >&2
  exit 2
fi

if [[ -f "$arg" ]]; then
  echo "Loading $arg"
  loaded="$(docker load -i "$arg")"
  echo "$loaded"
  # "Loaded image: ghcr.io/owner/repo:v1.2.3"
  ref="$(printf '%s\n' "$loaded" | sed -n 's/^Loaded image: //p' | tail -n 1)"
  if [[ -z "$ref" || "$ref" != *:* ]]; then
    echo "could not find a tagged image in $arg" >&2
    exit 1
  fi
  BOT_IMAGE="${ref%:*}"
  version="${ref##*:}"
else
  case "$arg" in
    latest | v*) version="$arg" ;;
    *)
      echo "$arg is neither a file nor a version tag (vX.Y.Z or latest)" >&2
      exit 2
      ;;
  esac
  echo "Pulling $BOT_IMAGE:$version"
  docker pull "$BOT_IMAGE:$version"
fi

echo "Image reports version $(docker run --rm "$BOT_IMAGE:$version" -version)"

mkdir -p data
export BOT_IMAGE BOT_VERSION="$version"
docker compose up -d
docker compose ps
