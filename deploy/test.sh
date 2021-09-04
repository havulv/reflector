#!/usr/bin/env bash

# Derived from the example in the chart-testing repo

set -o errexit
set -o nounset
set -o pipefail

readonly CT_VERSION=v3.4.0
readonly KIND_VERSION=v0.11.1
readonly CLUSTER_NAME=reflector-e2e

run_ct_container() {
  echo 'Running ct container...'
  docker run --rm --interactive --detach --network host --name ct \
    --volume "${HOME}/.kube/config:/root/.kube/config" \
    --volume "$(pwd):/workdir" \
    --workdir /workdir \
    "quay.io/helmpack/chart-testing:$CT_VERSION" \
    cat
  echo
}

cleanup() {
  echo 'Killing ct container...'
  docker kill ct > /dev/null 2>&1
  echo 'Done!'
}

docker_exec() {
  docker exec --interactive ct "$@"
}

install_kind() {
  echo 'Installing kind...'
  curl -sSLo kind "https://github.com/kubernetes-sigs/kind/releases/download/$KIND_VERSION/kind-linux-amd64"
  chmod +x kind
  sudo mv kind /usr/local/bin/kind
}

create_kind_cluster() {
  hash kind || install_kind
  if ! kind get clusters | grep "${CLUSTER_NAME}"; then
    kind create cluster --name "$CLUSTER_NAME" --config kind.yaml --wait 60s
  fi

  echo 'Cluster ready!'
  echo
}

install_charts() {
  docker_exec ls
  docker_exec bash -c "cd deploy && ct lint-and-install"
  echo
}

main() {
  run_ct_container
  trap cleanup EXIT

  create_kind_cluster
  install_charts
}

main
