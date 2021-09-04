#!/usr/bin/env bash

# Derived from the example in the chart-testing repo

set -o errexit
set -o nounset
set -o pipefail


readonly REGISTRY_NAME='kind-registry'
readonly REGISTRY_PORT='5000'
readonly CT_VERSION=v3.4.0
readonly KIND_VERSION=v0.11.1
readonly CLUSTER_NAME=reflector-e2e

WORKING_DIR=${PWD}
CLEANUP_CLUSTER=true

if [[ "$(basename "$WORKING_DIR")" == "deploy" ]]; then
  echo "Please run this from the root of the repo"
  exit 1
fi

if [ $# -gt 0 ]; then
  if [[ "$1" = "--dont-cleanup-cluster" ]]; then
    CLEANUP_CLUSTER=false
  fi
fi

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
  if [[ "${CLEANUP_CLUSTER}" == "true" ]]; then
    echo "Deleting cluster..."
    kind delete cluster --name $CLUSTER_NAME
    echo "Done!"
  fi
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
    kind create cluster --name "$CLUSTER_NAME" --config deploy/kind.yaml --wait 60s
    running="$(docker inspect -f '{{.State.Running}}' "${REGISTRY_NAME}" 2>/dev/null || true)"
    if [ "${running}" != 'true' ]; then
      docker run \
        -d --restart=always -p "127.0.0.1:${REGISTRY_PORT}:5000" --name "${REGISTRY_NAME}" \
        registry:2
    fi

    # connect the registry to the cluster network
    # (the network may already be connected)
    docker network connect "kind" "${REGISTRY_NAME}" || true

    # Document the local registry
    # https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
  fi

  echo 'Cluster ready!'
  echo
}

build_image() {
  docker build . -t localhost:5000/havulv/reflector:latest
  docker push localhost:5000/havulv/reflector:latest
}

install_charts() {
  docker_exec bash -c "cd deploy && ct lint-and-install --debug"
  echo
}

main() {
  run_ct_container
  trap cleanup EXIT SIGINT

  create_kind_cluster
  build_image
  install_charts
}

main
