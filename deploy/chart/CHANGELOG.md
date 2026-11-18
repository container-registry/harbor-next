# Changelog

## [2.2.0](https://github.com/container-registry/harbor-next/compare/chart-v2.1.2...chart-v2.2.0) (2026-11-18)


### Features

* Add Default Project To System Settings ([#1405](https://github.com/container-registry/harbor-next/issues/1405)) ([a851d11](https://github.com/container-registry/harbor-next/commit/a851d113145cc85c8ee410910312f25f50fef8f1))
* Add Model Sync As Beta Feature ([#1478](https://github.com/container-registry/harbor-next/issues/1478)) ([32e1f88](https://github.com/container-registry/harbor-next/commit/32e1f88f155f18532b19339fb2c1b068727088a5))
* **chart:** Add Optional In-Cluster PostgreSQL Subchart ([#1404](https://github.com/container-registry/harbor-next/issues/1404)) ([f5d7d1d](https://github.com/container-registry/harbor-next/commit/f5d7d1d721ff5861c7f756ae264b71ab9b03fea5))
* **chart:** Add Webhook Egress Policy Levels And Allowlist ([#1185](https://github.com/container-registry/harbor-next/issues/1185)) ([ba299a8](https://github.com/container-registry/harbor-next/commit/ba299a85a625f5cdb6fa17e4ae87f564491dbc34))


### Bug Fixes

* **chart:** extraTemplateManifests silently drops every document after the first ([#687](https://github.com/container-registry/harbor-next/issues/687)) ([a20dcfc](https://github.com/container-registry/harbor-next/commit/a20dcfc56809d01b2cce4a4d890857a00ab3c23e))
* **chart:** include service port in harbor.core.url helper ([#790](https://github.com/container-registry/harbor-next/issues/790)) ([#847](https://github.com/container-registry/harbor-next/issues/847)) ([43221eb](https://github.com/container-registry/harbor-next/commit/43221eb1aa31be10a572826fd3510891dc2d8a9d))
* **chart:** Rebuild Indexes On PostgreSQL Collation Version Mismatch ([#1413](https://github.com/container-registry/harbor-next/issues/1413)) ([133d1ea](https://github.com/container-registry/harbor-next/commit/133d1eafcc65a10f9a2747ad0e403f65370caafc))
* **chart:** Stop --reuse-values Upgrades From 2.15 Charts ([#1482](https://github.com/container-registry/harbor-next/issues/1482)) ([9713de5](https://github.com/container-registry/harbor-next/commit/9713de5b8f66c58c2092c921ad5f35d589cb3666))
* Skip empty component Secrets when identity is fully externalized ([#835](https://github.com/container-registry/harbor-next/issues/835)) ([cdeadab](https://github.com/container-registry/harbor-next/commit/cdeadab735008bfda0dd792eb00fd6b25e2d2a32))


### Harbor Releases

* release 2.16.0 ([#268](https://github.com/container-registry/harbor-next/issues/268)) ([2862ead](https://github.com/container-registry/harbor-next/commit/2862ead965368458e1192268828f524262cf8122))


### Chart Changes

* Add Argo CD example with pinned-secret Application setup ([#836](https://github.com/container-registry/harbor-next/issues/836)) ([af4898b](https://github.com/container-registry/harbor-next/commit/af4898bd2640af33f0e28b00a8b1b5dc77e783b0))

## [2.1.1](https://github.com/container-registry/harbor-next/compare/chart-v2.1.0...chart-v2.1.1) (2026-10-27)


### Bug Fixes

* **chart:** Bump Chart appVersion To v2.15.14 ([#1336](https://github.com/container-registry/harbor-next/issues/1336)) ([2c3a3ff](https://github.com/container-registry/harbor-next/commit/2c3a3ff63317b1a9f1e01d1d39b2c648c244325a))
* **chart:** Raise Exporter Probe Timeouts To 5s ([#1288](https://github.com/container-registry/harbor-next/issues/1288)) ([af6e5c0](https://github.com/container-registry/harbor-next/commit/af6e5c0f6905f306a5b08302c2a41ba647db7f7f))

## [2.1.0](https://github.com/container-registry/harbor-next/compare/chart-v2.0.0...chart-v2.1.0) (2026-10-19)


### Features

* **chart:** Add Harbor Grafana dashboard with 8gcr pgx monitoring ([#849](https://github.com/container-registry/harbor-next/issues/849)) ([572676e](https://github.com/container-registry/harbor-next/commit/572676e17868d52e4663c0bb4624732253b2021a))
* **chart:** Bump appVersion To v2.15.10 ([#1184](https://github.com/container-registry/harbor-next/issues/1184)) ([da299f0](https://github.com/container-registry/harbor-next/commit/da299f0cc56aae0a2d9681246aad88cf7b11bbd2))
* **chart:** Bump chart appVersion to v2.15.13 ([#1272](https://github.com/container-registry/harbor-next/issues/1272)) ([9cf5a5a](https://github.com/container-registry/harbor-next/commit/9cf5a5aac1cbf89b25764c8392dc767b64807926))


### Bug Fixes

* Send Cache-Control no-store On API And Static JSON Responses ([#992](https://github.com/container-registry/harbor-next/issues/992)) ([6f90be5](https://github.com/container-registry/harbor-next/commit/6f90be58edaf93482e6f5591b77212c06ad93afa))
* Send Referrer-Policy And Permissions-Policy From Portal And Core ([#994](https://github.com/container-registry/harbor-next/issues/994)) ([f8f658c](https://github.com/container-registry/harbor-next/commit/f8f658ce8430293abbb18fa4b69d2dffe62f5e7d))
* Send X-Content-Type-Options nosniff From Portal And Core ([#990](https://github.com/container-registry/harbor-next/issues/990)) ([08858a4](https://github.com/container-registry/harbor-next/commit/08858a48865715f352d93de9655f11456c134ae3))
* Send X-Frame-Options And CSP frame-ancestors From Portal And Core ([#989](https://github.com/container-registry/harbor-next/issues/989)) ([bbeb449](https://github.com/container-registry/harbor-next/commit/bbeb4498766854b36c63dfc704e4d4f16f95cc38))
* **webhook:** Block Private-Network Targets And Stop Reflecting Response Bodies ([#1140](https://github.com/container-registry/harbor-next/issues/1140)) ([ff8c122](https://github.com/container-registry/harbor-next/commit/ff8c12202c3d2487cb587d524cb0c82559f52268))

## [2.0.0](https://github.com/container-registry/harbor-next/compare/chart-v1.0.0...chart-v2.0.0) (2026-09-08)


### Features

* **chart:** Add production-ready Helm chart ([#56](https://github.com/container-registry/harbor-next/issues/56)) ([2f19897](https://github.com/container-registry/harbor-next/commit/2f19897b0bec27af1cc896b3621cb95ff5c27a35))


### Bug Fixes

* Use canonical Apache license URL in authoritative.go header ([#722](https://github.com/container-registry/harbor-next/issues/722)) ([a088001](https://github.com/container-registry/harbor-next/commit/a0880018fd51e7c96fdcfa2565a5c86c63f2f203))


### Code Refactoring

* Complete YS review and update to ys-0.2.31 ([#672](https://github.com/container-registry/harbor-next/issues/672)) ([3061c18](https://github.com/container-registry/harbor-next/commit/3061c1869ab921531421ba86ebb6806a5498775f))


### Chart Changes

* Rename Helm chart from harbor-next to harbor ([#813](https://github.com/container-registry/harbor-next/issues/813)) ([a71877e](https://github.com/container-registry/harbor-next/commit/a71877e31a5bb7d55d057f1ae510c2198474573a))

## 2.0.0 (2026-08-18)


### Features

* **chart:** Add production-ready Helm chart ([#56](https://github.com/container-registry/harbor-next/issues/56)) ([2f19897](https://github.com/container-registry/harbor-next/commit/2f19897b0bec27af1cc896b3621cb95ff5c27a35))
