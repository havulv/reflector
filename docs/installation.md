
# Installation

### Helm Chart

The standard method of installing the reflector is to use the Helm
chart located in the [chart directory](./deploy/chart). You can find
more specific instructions for installing the reflector in that
directory, but the quickstart is:

```bash
# Add the Havulv chart repository to Helm
$ helm repo add havulv https://charts.havulv.io

# Install the reflector chart
$ helm install reflector --namespace kube-system havulv/reflector
```

### Custom Install Requirements

For a custom installation there are two ways to deploy the reflector:
in cluster and out of cluster.

When using the reflector in cluster, it is important to give the
reflector a service account token which has access to the following
verbs for `secrets` in the relevant namespaces:
* `get`: for fetching a secret after the reflector is notified about a
  change in any secret.
* `watch`: for watching for updates to secrets.
* `update`: for updating secrets with new data.
* `list`: for listing secrets in namespaces, and selecting the secret
  that should be updated.
* `create`: for creating new secrets from the originating secret.
* (if using `cascadeDelete`) `delete`: for deleting secrets after the
  originating secret is deleted.

And the following verbs for the `namespaces` resource:
* `watch` for watching for updates to new namespaces.
* `list` for listing namespaces when `*` is used as a value for the
  namespace annotation.

Out of the cluster, the same considerations need to be applied for
permissioning, but you need only point the reflector at a kube config
with the `--config` command line parameter.


