# Changelog

All notable changes to this project will be documented in this file.

This changelog mirrors [GitHub Releases](https://github.com/container-registry/harbor-next/releases).

---

## v2.14.x

### v2.14.2 (2026-01-15)
Component updates and bug fixes including trivy adapter bump and search user/groups fixes.

### v2.14.1 (2025-11-24)
- Add max_upstream_conn parameter for proxy_cache projects
- UI for limit upstream registry connection
- Robot account fixes and audit log improvements

### v2.14.0 (2025-09-17)
**New Features:**
- **Enhanced Proxy-cache**: Syncs state with upstream registry by deleting local cache when artifacts are removed
- **Single Active Replication**: Prevents parallel runs under the same policy
- **Enhanced artifact scanning**: Support for fixVersion in CVE reports
- **Enhanced garbage collection**: Displays GC progress while running
- **Enhanced CNAI Model integration**: Support for raw CNAI model format
- **Russian language support**

**Breaking Changes:**
- Replication adapter whitelist introduced to define actively supported adapters

---

## v2.13.x

### v2.13.4 (2026-01-15)
Bug fixes including artifact_type column typo fix and trivy adapter bump.

### v2.13.3 (2025-11-24)
Component updates and build improvements.

### v2.13.2 (2025-07-31)
Component updates including ORM filter updates and trivy bump.

### v2.13.1 (2025-05-26)
Component updates including Helm Chart Copy Button fix and build improvements.

### v2.13.0 (2025-04-10)
**New Features:**
- **Audit log extension**: Enhanced granular tracking of user actions and system events
- **Enhanced OIDC**: Improved support for user session logout and PKCE
- **Integration with CloudNativeAI (CNAI)**: AI model management and processing capabilities
- **Redis TLS support**: Enhanced security for Redis communication
- **Enhanced Dragonfly Preheating**: New parameters and customizable scope

**Breaking Changes:**
- Updated CSRF key generation
- Removed with_signature parameter
- Project maintainers, developers, and guests do not have permission to list project logs

**Deprecations:**
- Removed robotV1 from code base

---

## v2.12.x

### v2.12.4 (2025-05-23)
Component updates and build fixes.

### v2.12.3 (2025-05-07)
Component updates including UI fixes and trivy adapter pin.

### v2.12.2 (2025-01-16)
Base image updates.

### v2.12.1 (2024-12-24)
Bug fixes including robot deletion event and export CVE permission fixes.

### v2.12.0 (2024-11-08)
**New Features:**
- **Enhanced robot account**: Additional configuration options for better CI/CD integration
- **Speed limit of proxy cache project**: Control network speed when pulling from proxy cache
- **Enhanced LDAP on-boarding process**: Improved user login performance
- **Integration with ACR & ACR EE Registry**: Seamless image replication
- **SBOM Generation and Management**: Generate, view, download, and replicate SBOMs

---

## v2.11.x

### v2.11.2 (2024-11-19)
Component updates including golang bump and beego upgrade.

### v2.11.1 (2024-08-21)
Cherry-pick fixes including artifact accessory URL and scan button fixes.

### v2.11.0 (2024-06-06)
**New Features:**
- **SBOM Generation and Management**: Manual or automatic SBOM generation
- **Supporting OCI Distribution Spec v1.1.0**
- **Integration with VolcEngine Registry**
- **Korean UI Translation**

---

## v2.10.x

### v2.10.3 (2024-07-04)
Component updates and bug fixes.

### v2.10.2 (2024-04-10)
Bug fixes including retention task panic fix.

### v2.10.1 (2024-03-15)
Bug fixes including quota permissions and limited guest repository access.

### v2.10.0 (2023-12-19)
**New Features:**
- **Robot Account Full Access**: User-friendly tutorial for robot creation with customizable permissions
- **Supporting OCI Distribution Spec v1.1.0-rc3**
- **Quota Sorting**: Enable storage sorting in quota management
- **OIDC provider name customization**
- **Large-size blob support**: Uploads up to 128GB by default
- **GDPR compliant audit logs**

---

## v2.9.x

### v2.9.5 (2024-07-01)
Component updates and bug fixes.

### v2.9.4 (2024-04-18)
Component updates including trivy bump and golang upgrade.

### v2.9.3 (2024-03-08)
Component updates including IP family config and strong SSL ciphers.

### v2.9.2 (2024-01-29)
Bug fixes including scanner skip update pull time and accessory ordering.

### v2.9.1 (2023-11-02)
Component updates including redis batch job listing and trivy bump.

### v2.9.0 (2023-09-01)
**New Features:**
- **Security Hub**: Security insights including scanned/unscanned artifacts and vulnerability search
- **GC Enhancements**: Detailed execution history and parallel deletion
- **Supporting OCI Distribution Spec v1.1.0-rc2**: Notation signature and Nydus conversion support
- **Customized banner message**
- **Quota Update Provider**: Redis-based optimistic locking for quota updates

**Deprecations:**
- **Removal of Notary**: No longer included in UI or backend

**Breaking Changes:**
- Only PostgreSQL >= 12 supported for external databases

---

## v2.8.x

### v2.8.6 (2024-04-22)
Component updates and bug fixes.

### v2.8.5 (2024-03-07)
Bug fixes including beego max memory increase and URL limit to local site.

### v2.8.4 (2023-08-16)
Component updates including redis keys scan migration and cache db customization.

### v2.8.3 (2023-07-28)
Component updates including gitlab adapter fix and trivy bump.

### v2.8.2 (2023-06-05)
Bug fixes including proxy cache pull time and 429 error handling.

### v2.8.1 (2023-05-12)
Bug fixes including list artifacts performance improvement.

### v2.8.0 (2023-04-17)
**New Features:**
- **Supporting OCI Distribution Spec v1.1.0-rc1**: Referrers API
- **CloudEvents format for webhooks**
- **Jobservice Dashboard Phase 2**: Logs for running tasks, cleanup expired executions
- **Option to Skip Update Pull Time for Scanner**
- **Primary auth method from Identity Provider**

**Deprecations:**
- **Removal of ChartMuseum**: No longer included in UI or backend

---

## v2.7.x

### v2.7.4 (2023-11-30)
Component updates including golang and trivy bumps.

### v2.7.3 (2023-09-11)
Bug fixes including list artifacts performance and redis keys scan migration.

### v2.7.2 (2023-04-25)
Bug fixes including copy artifact and retention webhook fixes.

### v2.7.1 (2023-02-21)
Bug fixes including schedule list and retention/immutable API fixes.

### v2.7.0 (2022-12-19)
**New Features:**
- **Jobservice monitor**: Dashboard to monitor and control job queues/schedules/workers
- **Replication by chunk**: Copy over chunk when copying image blobs
- **JFrog Artifactory as Proxy-Cache source**
- **OIDC group filter**
- **Session timeout customization**

**Deprecations:**
- Chartmuseum deprecation (removal in v2.8.0)
- Notary deprecation (removal in v2.8.0)
- Email configuration removed
- PostgreSQL 9.6 support dropped

---

## v2.6.x

### v2.6.4 (2023-02-22)
Bug fixes including retention/immutable API and user password reset fixes.

### v2.6.3 (2023-01-05)
Bug fixes including RedHat registry proxy cache fix.

### v2.6.2 (2022-11-10)
Added copy-by-chunk for replication and registry HTTP client timeout customization.

### v2.6.1 (2022-10-11)
Bug fixes including sentinel redis URL parsing and audit log forward fixes.

### v2.6.0 (2022-08-29)
**New Features:**
- **Cache Layer**: Improved performance for pulling artifacts in high concurrency
- **CVE Export**: Export vulnerability data for artifacts
- **Purge AuditLog**: Periodic purge and remote syslog forwarding
- **Backup/Restore with Velero**
- **GDPR compliant user deletion**
- **WebAssembly artifact support** (Experimental)
- **GitHub GHCR as proxy cache**

---

## v2.5.x

### v2.5.6 (2023-02-23)
Bug fixes including retention/immutable API and trivy bump.

### v2.5.5 (2023-01-16)
Bug fixes including RedHat registry proxy cache fix.

### v2.5.4 (2022-08-29)
Bug fixes including robot update regression and docker compose v2 support.

### v2.5.3 (2022-07-08)
Bug fixes including execution status repair.

### v2.5.2 (2022-06-30)
Bug fixes including jobservice hook retry and retention policy update.

### v2.5.1 (2022-05-30)
Bug fixes including GC history update time and accessory count fixes.

### v2.5.0 (2022-04-11)
**New Features:**
- **Cosign Artifact Signing and Verification**: Sigstore/Cosign support for artifact signing
- Improved performance for concurrent pull requests
- Improved GC failure tolerance
- Replication skip for proxy cache projects
- Distribution upload purging

**Breaking Changes:**
- Only PostgreSQL >= 10 supported for external databases

---

## v2.4.x

### v2.4.3 (2022-08-03)
Bug fixes including retention policy and robot account update fixes.

### v2.4.2 (2022-03-17)
Bug fixes including LDAP user group privileges and GC failure tolerance.

### v2.4.1 (2021-12-17)
Bug fixes including user groups pagination and RSA key format fix.

### v2.4.0 (2021-10-28)
**New Features:**
- **Distributed tracing**: Enhanced troubleshooting and performance identification
- Replication with Robot Account
- Stop scan jobs
- Replication exclusions and rate limits
- OIDC auth based user deletion
- Trivy 0.20 with go.sum scanning

**Deprecations:**
- Legacy robot account removed
- Limited ChartMuseum support

---

## v2.3.x

### v2.3.5 (2021-12-15)
Bug fixes.

### v2.3.4 (2021-11-11)
Bug fixes.

### v2.3.3 (2021-09-28)
Bug fixes.

### v2.3.2 (2021-08-23)
Bug fixes.

### v2.3.1 (2021-07-23)
Bug fixes.

### v2.3.0 (2021-06-21)
**New Features:**
- **Declarative Config**: Environment variables to overwrite Harbor configuration
- **IPv6 support**: Running on IPv6-only infrastructure
- **Photon 4.0 upgrade**: PostgreSQL v13.3, Redis v6.0.13
- Jobservice metrics
- Destination namespace flattening for replication
- Trivy 0.17 with JAR/WAR/EAR and Go binary scanning

---

## v2.2.x

### v2.2.4 (2021-10-25)
Bug fixes.

### v2.2.3 (2021-07-07)
Bug fixes.

### v2.2.2 (2021-05-20)
Bug fixes.

### v2.2.1 (2021-03-30)
Bug fixes.

### v2.2.0 (2021-02-24)
**New Features:**
- **System Level Robot Account**: Access multiple projects with selective API access
- **Metrics & Observability**: Performance and system information indicators
- **OIDC Admin Group**: Privileged admin group for OIDC auth
- **Aqua CSP Scanner support**
- Proxy cache for GCR, ECR, Azure, Quay.io
- Dell EMC ECS s3 support

**Deprecations:**
- Built-in Clair deprecated

---

## v2.1.x

### v2.1.6 (2021-07-09)
Bug fixes.

### v2.1.5 (2021-04-28)
Bug fixes.

### v2.1.4 (2021-03-16)
Bug fixes.

### v2.1.3 (2021-01-11)
Bug fixes.

### v2.1.2 (2020-12-14)
Bug fixes.

### v2.1.1 (2020-10-28)
Bug fixes.

### v2.1.0 (2020-09-18)
**New Features:**
- **Non-blocking Garbage Collection**: Continue pushing/pulling during GC
- **Proxy Cache**: Pull through cache for Dockerhub and Harbor
- **P2P Preheat**: Integration with Alibaba Dragonfly and Uber Kraken
- **Harbor for AI/ML**: Kubeflow datamodels support
- **Sysdig Image Scanner support**

---

## v2.0.x

### v2.0.6 (2021-02-05)
Bug fixes.

### v2.0.5 (2020-12-10)
Bug fixes.

### v2.0.4 (2020-11-23)
Bug fixes.

### v2.0.3 (2020-09-22)
Bug fixes.

### v2.0.2 (2020-08-04)
Bug fixes.

### v2.0.1 (2020-06-30)
Bug fixes.

### v2.0.0 (2020-05-13)
**New Features:**
- **OCI compliant cloud native artifact support**: OCI images, image indexes, multi-arch images
- **Trivy as default scanner**
- **TLS between Harbor components**
- **Webhook enhancements**: Slack support, selectable events, multiple endpoints
- **Robot account expiration**: Individual expiration time per robot
- View and manage untagged images in UI

**Breaking Changes:**
- REST APIs use `/api/v2.0` prefix
- Default configuration file renamed to `harbor.yml.tmpl`
- Project quota based on image count removed
- CRON schedule follows UTC timezone

---

## v1.10.x

### v1.10.19 (2024-09-20)
Security and bug fixes.

### v1.10.18 (2023-06-05)
Security and bug fixes.

### v1.10.17 (2023-03-02)
Security and bug fixes.

### v1.10.16 (2023-02-06)
Security and bug fixes.

### v1.10.15 (2022-11-22)
Security and bug fixes.

### v1.10.14 (2022-09-30)
Security and bug fixes.

### v1.10.13 (2022-08-26)
Security and bug fixes.

### v1.10.12 (2022-08-04)
Security and bug fixes.

### v1.10.11 (2022-05-10)
Security and bug fixes.

### v1.10.10 (2022-01-12)
Security and bug fixes.

### v1.10.9 (2021-10-28)
Security and bug fixes.

### v1.10.8 (2021-06-30)
Security and bug fixes.

### v1.10.7 (2021-05-28)
Security and bug fixes.

### v1.10.6 (2020-11-19)
Security and bug fixes.

### v1.10.5 (2020-09-15)
Security and bug fixes.

### v1.10.4 (2020-07-15)
Security and bug fixes.

### v1.10.3 (2020-06-11)
Security and bug fixes.

### v1.10.2 (2020-04-09)
Security and bug fixes.

### v1.10.1 (2020-02-14)
Security and bug fixes.

### v1.10.0 (2019-12-13)
**New Features:**
- **Pluggable Scanners**: Aqua Security and Anchore scanner support
- **Tag Immutability**: Prevent overwriting images with matching tags
- **Replication enhancements**: Gitlab, Quay.io, JFrog Artifactory support
- **OIDC groups** and user-defined CLI secrets
- **Limited Guest role**: Lower permissions than Guest
- **Project quota exceeded webhook**

---

## v1.9.x

### v1.9.4 (2019-12-31)
Bug fixes.

### v1.9.3 (2019-11-18)
Bug fixes.

### v1.9.2 (2019-11-05)
Bug fixes.

### v1.9.1 (2019-10-15)
Bug fixes.

### v1.9.0 (2019-09-19)
**New Features:**
- **Project Quotas**: Limit artifacts or storage per project
- **Tag Retention**: Rules to retain/remove tags based on criteria
- **Webhooks**: Integration for push, pull, delete, scan events
- **CVE whitelists**: Exception policies for certain CVEs
- **Replication enhancements**: GCR, Azure, ECR, Alibaba Cloud, Helm Hub support
- Groups privileges prioritization
- External syslog endpoint configuration
- Non-root container security enhancement
- Robot accounts for chart upload/fetch

---

## v1.8.x

### v1.8.6 (2019-11-18)
Bug fixes.

### v1.8.5 (2019-11-05)
Bug fixes.

### v1.8.4 (2019-10-15)
Bug fixes.

### v1.8.3 (2019-09-18)
Bug fixes.

### v1.8.2 (2019-08-14)
Bug fixes.

### v1.8.1 (2019-06-17)
Bug fixes.

### v1.8.0 (2019-05-21)
[Full list of issues fixed in v1.8.0](https://github.com/goharbor/harbor/issues?q=is%3Aissue+is%3Aclosed+label%3Atarget%2F1.8.0)
* Support for OpenID Connect - OpenID Connect (OIDC) is an authentication layer on top of OAuth 2.0, allowing Harbor to verify the identity of users based on the authentication performed by an external authorization server or identity provider.
* Robot accounts - Robot accounts can be configured to provide administrators with a token that can be granted appropriate permissions for pulling or pushing images. Harbor users can continue operating Harbor using their enterprise SSO credentials, and use robot accounts for CI/CD systems that perform Docker client commands.
* Replication advancements - Harbor new version replication allows you to replicate your Harbor repository to and from non-Harbor registries. Harbor 1.8 expands on the Harbor-to-Harbor replication feature, adding the ability to replicate resources between Harbor and Docker Hub, Docker Registry, and Huawei Registry. This is enabled through both push and pull mode replication.
* Health check API, showing detailed status and health of all Harbor components.
* Support for defining cron-based scheduled tasks in the Harbor UI. Administrators can now use cron strings to define the schedule of a job. Scan, garbage collection and replication jobs are all supported.
  API explorer integration. End users can now explore and trigger Harbor's API via the swagger UI nested inside Harbor's UI.
* Introduce a new master role to project, the role's permissions are more than developer and less than project admin.
* Introduce harbor.yml as the replacement of harbor.cfg and refactor the prepare script to provide more flexibility to the installation process based on docker-compose
* Enhancement of the Job Service engine to include webhook events, additional APIs for automation, and numerous bug fixes to improve the stability of the service.
* Docker Registry upgraded to v2.7.1.

---

## Historical Releases (v1.7.x and earlier)

### v1.7.5 (2019-04-02)
* Bumped up Clair to v2.0.8
* Fixed issues in supporting windows images. #6992 #6369
* Removed user-agent check-in notification handler. #5729
* Fixed the issue global search not working if chartmuseum is not installed #6753

### v1.7.4 (2019-03-04)
[Full list of issues fixed in v1.7.4](https://github.com/goharbor/harbor/issues?q=is%3Aissue+is%3Aclosed+label%3Atarget%2F1.7.4)

### v1.7.1 (2019-01-07)
[Full list of issues fixed in v1.7.1](https://github.com/goharbor/harbor/issues?q=is%3Aissue+is%3Aclosed+label%3Atarget%2F1.7.1)

### v1.7.0 (2018-12-19)
* Support deploy Harbor with Helm Chart, enables the user to have high availability of Harbor services, refer to the [Installation and Configuration Guide](https://github.com/goharbor/harbor-helm/tree/1.0.0).
* Support on-demand Garbage Collection, enables the admin to configure run docker registry garbage collection manually or automatically with a cron schedule.
* Support Image Retag, enables the user to tag image to different repositories and projects, this is particularly useful in cases when images need to be retagged programmatically in a CI pipeline.
* Support Image Build History, makes it easy to see the contents of a container image, refer to the [User Guide](https://github.com/goharbor/harbor/blob/release-1.7.0/docs/user_guide.md#build-history).
* Support Logger customization, enables the user to customize STDOUT / STDERR / FILE / DB logger of running jobs.
* Improve the user experience of Helm Chart Repository:
  - Chart searching is included in the global search results
  - Show the total number of chart versions in the chart list
  - Mark labels in helm charts
  - The latest version can be downloaded as default one on the chart list view
  - The chart can be deleted by deleting all the versions under it


### v1.6.0 (2018-09-11)

- Support manages Helm Charts: From version 1.6.0, Harbor is upgraded to be a composite cloud-native registry, which supports both image management and helm charts management.
- Support LDAP group: User can import an LDAP/AD group to Harbor and assign project roles to it.
- Replicate images with label filter: Use newly added label filter to narrow down the sourcing image list when doing image replication.
- Migrate multiple databases to one unified PostgreSQL database.

### v1.5.0 (2018-05-07)

- Support read-only mode for registry: Admin can set registry to read-only mode before GC. [Details](https://github.com/vmware/harbor/blob/master/docs/user_guide.md#managing-registry-read-only)
- Label support: User can add label to image/repository, and filter images by label on UI/API. [Details](https://github.com/vmware/harbor/blob/master/docs/user_guide.md#managing-labels)
- Show repositories via Cardview.
- Re-work Job service to make it HA ready.

### v1.4.0 (2018-02-07)

- Replication policy rework to support wildcard, scheduled replication.
- Support repository level description.
- Batch operation on projects/repositories/users from UI.
- On board LDAP user when adding a member to a project.

### v1.3.0 (2018-01-04)

- Project level policies for blocking the pull of images with vulnerabilities and unknown provenance.
- Remote certificate verification of replication moved to target level.
- Refined all images to improve security.

### v1.2.0 (2017-09-15)

- Authentication and authorization, implementing vCenter Single Sign On across components and role-based access control at the project level. [Read more](https://vmware.github.io/vic-product/assets/files/html/1.2/vic_overview/introduction.html#projects)
- Full integration of the vSphere Integrated Containers Registry and Management Portal user interfaces. [Read more](https://vmware.github.io/vic-product/assets/files/html/1.2/vic_cloud_admin/)
- Image vulnerabilities scanning.

### v1.1.0 (2017-04-18)

- Add in Notary support
- User can update the configuration through Harbor UI
- Redesign of Harbor's UI using Clarity
- Some changes to API
- Fix some security issues in the token service
- Upgrade the base image of nginx to the latest openssl version
- Various bug fixes.

### v0.5.0 (2016-12-6)

- Refactor for a new build process
- Easier configuration for HTTPS in prepare script
- Script to collect logs of a Harbor deployment
- User can view the storage usage (default location) of Harbor.
- Add an attribute to disable normal users from creating projects.
- Various bug fixes.

For Harbor virtual appliance:

- Improve the bootstrap process of ova installation.
- Enable HTTPS by default for .ova deployment, users can download the default root cert from UI for docker client or VCH.
- Preload a photon:1.0 image to Harbor for users who have no internet connection.

### v0.4.5 (2016-10-31)

- Virtual appliance of Harbor for vSphere.
- Refactor for new build process.
- Easier configuration for HTTPS in prepare step.
- Updated documents.
- Various bug fixes.

### v0.4.0 (2016-09-23)

- Database schema changed, data migration/upgrade is needed for previous version.
- A project can be deleted when no images and policies are under it.
- Deleted users can be recreated.
- Replication policy can be deleted.
- Enhanced LDAP authentication, allowing multiple uid attributes.
- Pagination in UI.
- Improved authentication for remote image replication.
- Display release version in UI
- Offline installer.
- Various bug fixes.

### v0.3.5 (2016-08-13)

- Vendoring all dependencies and remove go get from dockerfile
- Installer using Docker Hub to download images
- Harbor base images moved to Photon OS (except for official images from third party)
- New Harbor logo
- Various bug fixes

### v0.3.0 (2016-07-15)

- Database schema changed, data migration/upgrade is needed for previous version.
- New UI
- Image replication across multiple registry instances
- Integration with registry v2.4.0 to support image deletion and garbage collection
- Database migration tool
- Bug fixes

### v0.1.1 (2016-04-08)

- Refactored database schema
- Migrate to docker-compose v2 template
- Update token service to support layer mount
- Various bug fixes

### v0.1.0 (2016-03-11)

Initial release, key features include

- Role based access control (RBAC)
- LDAP / AD integration
- Graphical user interface (GUI)
- Auditing and logging
- RESTful API
- Internationalization
