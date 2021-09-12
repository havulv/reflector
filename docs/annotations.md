
# Annotations

```yaml
reflector.havulv.io/reflect: "true"
reflector.havulv.io/namespaces: "some,namespace,to,reflect,to"
```

The first annotation (`reflector.havulv.io/reflect: "true"`) indicates
that this secret should be reflected. If, at any point in the secret's
lifecycle, you wish to stop reflecting this secret, then remove the
annotation from the object. Note that the reflected secrets will not be
removed or updated if you remove this annotation from the originating
secret.

The second annotation (
`reflector.havulv.io/namespaces:"some,namespace,to,reflect,to"`)
indicates the namespaces that the secret should be reflected to. The
annotation's value is a comma separated list of the namespaces that
should be reflected to. If you supply an asterisk `*` as the value,
then the reflector will reflect the secret to every namespace that it
can.

One potential _gotcha_ related to this annotation, is the fact that,
when namespaces are updated, secrets will not be removed from
namespaces they are already reflected to.

For example, if the namespace annotation starts as
`kube-system,monitoring,logging` and then it is updated to
`kube-system,monitoring`, the secret in `logging` will not be removed.
Additionally, the `secret` in `logging` will not be updated when
changes to the originating secret occur.

In the generated secret, you can see that the two `reflector.havulv.io`
prefixed annotations from the originating secret have been removed and
replaced with four new ones:


###### `reflector.havulv.io/hash`

Is a hash of the originating secret, minus the reflection annotations
(`reflector.havulv.io/.*`). We use this to reduce the number of
needless updates to existing secrets when nothing has changed on the
secret except the reflection meta annotations. In practice, this only
affects the namespaces annotation: not updating secrets that have
already been reflected when a namespace is added or removed.

This is due to the fact that if `reflector.havulv.io/reflect` is
changed then the secret will never be reflected, and the hash will
not be checked.


###### `reflector.havulv.io/owner`

Is the ownership of the reflected secret. If the annotation is set to
anything other than `reflector` then the secret will not be updated on
changes to the originating secret.

This is useful in the case that a reflected secret needs some manual
tuning for a specific use case, or if some debugging in that namespace
needs to occur.


###### [EXPERIMENTAL] `reflector.havulv.io/reflected-at`

Is a timestamp in UNIX nanoseconds (UTC timezone) of when the secret
was first reflected. This is useful almost purely for debugging
purposes, and for comparing to apiserver audit logs. This annotation
is experimental, and may be removed at any point in the future. Please
try not to depend on this annotation.


###### [EXPERIMENTAL] `reflector.havulv.io/reflected-from`

Is the namespace that the originating secret exists in. This is useful
for debugging purposes, but is currently not used by the reflector.
This annotation is experimental and, in the future, this annotation be
removed.

Note that, the reflector makes an assumption that only one secret
will be globally reflected at one time. That is, there will only be
one secret named `my-secret` with the reflection annotations in all
namespaces at any given time.

Consider the following example of this assumption: if you have one
secret named `my-secret` in the namespace `kube-system` and another
secret named `my-secret` in `default` and both have the annotation
`reflector.havulv.io/namespaces: "monitoring"`, then the resulting
secret named `my-secret` in `monitoring` is undefined.

The reflection of the secret in `default` may clobber the secret
reflected from `kube-system` or vice-versa, it will depend on the order
in which they were created or updated, but it is not up to the
reflector to resolve this conflict.  In the future, there may be
functionality which will prevent the secrets from colliding and produce
an error log (this annotation will be helpful, in that case), but this
has not been implemented yet.
