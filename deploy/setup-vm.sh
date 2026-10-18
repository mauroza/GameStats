#!/usr/bin/env bash
# Prepara una VM Ubuntu 24.04 para GameStats (se ejecuta una sola vez).
# Uso, desde tu computadora:
#   ssh usuario@IP 'bash -s' < deploy/setup-vm.sh
set -euo pipefail

echo "==> Instalando Docker y Compose desde los repositorios de Ubuntu"
sudo apt-get update -y
sudo apt-get install -y docker.io docker-compose-v2 curl
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"

# La VM pequeña (1 GB) necesita swap para construir/levantar la JVM sin quedarse sin memoria.
if ! swapon --show | grep -q /swapfile; then
  echo "==> Creando 2 GB de swap"
  sudo fallocate -l 2G /swapfile
  sudo chmod 600 /swapfile
  sudo mkswap /swapfile
  sudo swapon /swapfile
  echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab > /dev/null
fi

mkdir -p "$HOME/gamestats"
echo "==> Listo. Docker $(docker --version | cut -d' ' -f3) instalado."
echo "    Cierra y vuelve a abrir la sesión SSH para usar docker sin sudo."
