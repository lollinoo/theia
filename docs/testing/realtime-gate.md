# Realtime PR Gate

Run the required five-job gate locally from the repository root before opening or updating a PR:

```sh
make backend-fast frontend-fast realtime-stress collector-contract browser-e2e
```

The five required gate commands are:

```sh
make backend-fast
make frontend-fast
make realtime-stress
make collector-contract
make browser-e2e
```

`backend-fast` runs backend vet, build, tests, and the enforced Go coverage threshold.

`frontend-fast` runs the frontend coverage suite, typecheck, and production build.

`realtime-stress` runs the deterministic backend stress coverage tests.

`collector-contract` runs the collector contract coverage tests against the SNMP fixtures.

`browser-e2e` installs Playwright Chromium and runs the real-browser Playwright suite.
