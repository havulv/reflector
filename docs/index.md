
# Reflector

`Reflector` is a kubernetes secrets reflector: taking secrets
with specific annotations from one namespace and maintaining updates
and copies of those secrets in other namespaces.

## Reflecting

The reflection is based on the annotations set on the secrets object.
To specify that a secret should be reflected, add the following
annotations:

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

In full, a secret that should be reflected may look like this:
```yaml
apiVersion: v1
kind: Secret
metadata:
  annotations:
    reflector.havulv.io/reflect: "true"
    reflector.havulv.io/namespaces: "monitoring"
    custom.annotation.k8s.io: "very-custom"
  labels:
    app.kubernetes.io/name: "some-application"
    app.kubernetes.io/component: "a-component"
  name: some-secret
  namespace: kube-system
data:
  username: YWRtaW4=
  password: MWYyZDFlMmU2N2Rm
type: Opaque
```

This secret will generate the following secret in the `monitoring`
namespace:

```yaml
apiVersion: v1
kind: Secret
metadata:
  annotations:
    reflector.havulv.io/hash: "c18d547cafb43e30a993439599bd08321bea17bfedbe28b13bce8a7f298b63a2"
    reflector.havulv.io/owner: "reflector"
    reflector.havulv.io/reflected-at: "1631380645000"
    reflector.havulv.io/reflected-from: "kube-system"
    custom.annotation.k8s.io: "very-custom"
  labels:
    app.kubernetes.io/name: "some-application"
    app.kubernetes.io/component: "a-component"
  name: some-secret
  namespace: monitoring
data:
  username: YWRtaW4=
  password: MWYyZDFlMmU2N2Rm
type: Opaque
```


