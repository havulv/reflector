

# Architecture


The reflector works in three major parts:

1. A syncing watcher component
2. A syncing endpoint component

## Syncer

The syncer runs through the following process on startup:

1. The syncer fetches a list of namespaces and secrets that it has access to
2. If sharding is enabled, the syncer will
3. If sharding is enabled, the syncer will then collect the secrets it is chosen to watch over
4. The syncer will set a watch on all the namespaces it has access to for new secrets that come into existence
  * If a new secret comes into being, and sharding is enabled, the syncer will consult its partition key as to whether it should watch it
5. Once the syncer is watching the namespaces, it will walk the namespaces and check for any secret with relevant annotations
  * `reflector.havulv.io/*`
6. Additionally, when sharding, the syncer will add an annotation to establish ownership of the secret (and a healthcheck ttl):
  * `reflector.havulv.io/checked-by: "${pod-hash} (${UNIX TIMESTAMP})`
  * You can tune the healthcheck ttl on the command line via `--shard-ttl`
6. After gathering all the secrets, the syncer will establish a tree of secrets and prune any secrets it doesn't need (e.g. if sharding is activated)

Once the syncer is watching a specific secret, it will do the following on various events:

### On Delete

If the `--cascade-deletion` flag is set, then the syncer will delete all of the synced secrets.

### On Change

On change, the secret syncer will read the `reflector.havulv.io/namespaces` and look for any  `reflector.havulv.io/reflect: true` annotations in order to establish
which namespaces the secret should be copied to. Each namespace should be separated by a comma. If the field is
empty then no action will be taken. If the field is `*` then all namespaces will be reflected to. So, to
summarize:
* `"this,that"` will reflect to the namespaces `this` and `that`
* `"*"` will reflect to all namespaces
* `""` will reflect to no namespaces

The syncer will then create the reflected secrets in the specified namespaces, adding the following annotations and
labels:

_Labels_:
* `reflector.havulv.io/reflected: true`

_Annotations_:
* `reflector.havulv.io/reflected-from: ${namespace the secret was reflected from}`
* `reflector.havulv.io/reflected-at: ${Unix timestamp of when the secret was reflected}`
* `reflector.havulv.io/hash: ${hash sum of the secret to speed up comparisons}`
