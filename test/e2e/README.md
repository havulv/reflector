
# End to End tests

This directory is for tests which verify that the reflector
will work correctly within the cluster. Currently, the tests
are quite basic, where the only verification is that the chart
will correctly install with the proper defaults.

This is not a true end to end test, but that will only come
after building out a framework (likely in Go instead of Bash)
which will install the chart, and then run through various
scenarios:

* The service account token works, and has the correct permissions.

* When a secret exists in the cluster with the correct annotations
  and then the reflector is installed, these secrets are reflected.

* Secrets reflected and then updated at the central secret are
  then updated, with a new hash.

* Secrets reflected and then ownership changed will not be updated
  on subsequent changes.

* Cascade delete set to true will delete the relevant secrets when
  the original secret is deleted.

* [Not yet implemented] Update limits are honored, when the reflector
  decides it is overloading the apiserver or updating too quickly.

* [Not yet implemented] Multiple sharded reflectors select secrets
  // namespaces that are mutually exclusive, even after restarts.
