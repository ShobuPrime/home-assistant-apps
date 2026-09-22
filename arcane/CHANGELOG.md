# Changelog

## 2.13.1

_2026-09-22_

> [!NOTE]
> This release fixes project directory creation, container update, and upgrade workflows.
> It also improves backup coverage and updates the Compose example configuration.

### Project and Compose fixes

- Create missing project bind directories as the runtime user — `8bcb59d538a28bcd149b42bcbb9e096d10eb1059` (#4138) (`kmendell`)
- Update the Compose example to include `DISTRIBUTION=1` — `1b21e5c3594d3e60d8f8a449981a078a989790d6` (`kmendell`)

### Container update and backup fixes

- Allow manual updates for containers excluded from automatic updates — `6c227d02de0f412371f4124214934af2933fe7f5` (#4139) (`kmendell`)
- Preserve data volumes when recreating containers during upgrades — `75f6ddb2bb8cee8cec52259d11d5123fe7b97f4d` (#4141) (`kmendell`)
- Include separately mounted projects in system backups — `081e638de8000ec033a7fe4b4ac036ff8571e238` (#4144) (`kmendell`)

---


## 2.13.0

_2026-09-20_

> [!NOTE]
> This release adds richer log controls, Telegram topic destinations, bulk container updates, retained logs, and project editor preferences.
> It also improves update workflows, registry handling, UI behavior, and performance across projects, activity, and image updates.

## Features

- Added a stream label toggle for distinguishing STDOUT and STDERR in logs. (#3976, `4f10dee49f7a3c4b32a695cc9f05584403ea674d`) — `kmendell`
- Added Telegram topic destinations for notifications. (#3978, `e47164421a22b8a9f12714a923be566889d2cbe1`) — `kmendell`
- Added a timestamp toggle to the log viewer controls. (#3979, `9fa57c867b1085a7142d7e755f87ddd9318b1a1d`) — `kmendell`
- Added personal defaults for the project editor layout. (#3986, `67b34b9c032d8f56379ed60322839d39dc5af855`) — `kmendell`
- Added bulk updates for selected containers. (#3971, `0365c6861d50ef5fc5aaf7f816e370955eadc398`) — `mewajda`
- Added retained log viewing for stopped projects. (#4097, `b350d394cd19027e90bb8dd581474de54745c434`) — `kmendell`

## Fixes and UI improvements

- Nested Compose services and networks under their respective sections in the outline. (#3985, `f4573188664d88d6d3a8ac0427c34611211dcb72`) — `angst911`
- Let the backend generate run IDs for manually submitted jobs. (#3987, `af96a6e99b3fdc0b64cd8b26552e624be7732e54`) — `kmendell`
- Added a separate tag-listing timeout for large registries. (#3990, `5262d2f33af966b856a10b8572c061bf7ba8fc18`) — `kmendell`
- Updated the sidebar update item. (#4003, `476d8b17fc4f695e90a2758740fa77acf8ab42aa`) — `kmendell`
- Sorted containers globally by live CPU and memory usage. (#4099, `18e4e3f5b1b2d8abf3999008cf3a0934bae6d4f3`) — `kmendell`
- Surfaced Update All startup errors and adopted already-running jobs. (#4104, `dc89ea0b1920f4f2d6eefb92f2cafb9b528822ca`) — `kmendell`
- Sorted network subnet and gateway columns numerically. (#4119, `324adf3585ece4ecd1f4f3317875521398028ea7`) — `kmendell`
- Prevented header actions from overlapping badges and added per-device layout modes. (#4121, `9454ce57db6763263e4628cc58a4c2e1c169183d`) — `kmendell`
- Included container image references in update discovery. (#4126, `e0357ec8468fadb8a8daba7804ec20594092d3b0`) — `kmendell`
- Displayed project-scoped versions instead of image digests. (#4127, `ddead64b8b96e0929a0e79acd8d1a4f350e2f5a3`) — `kmendell`

## Performance

- Skipped repeated per-message visibility reads and source lookups on the local activity stream. (#4105, `423cd549b74271f8a4b3992cf04e5920d6a06dc5`) — `kmendell`
- Resolved GitOps Compose paths once per project list request. (#4106, `f5d6f37dba1c9cab72271d6e9d183cbe1f7fd79e`) — `kmendell`
- Enriched filtered project lists after pagination. (#4108, `92349aa97949e54ee2409a1c4a3c35758df60e8a`) — `kmendell`
- Deduplicated and parallelized container tag checks. (#4109, `57812f8f0511e9f2d3f6b6fa17242da6db98d592`) — `kmendell`
- Replaced repeated update-record scans with indexed lookups. (#4110, `109f22402d059167b079f735b19b72e4029ca4ef`) — `kmendell`

---


## 2.12.0

_2026-09-16_

> [!NOTE]
> This release adds more flexible Git-based backups, improved vulnerability scanning, and expanded container and network visibility. It also fixes several authentication, permissions, deployment, and agent-related issues.

## Backups and Git sync

- Added **Back up to Git** sync mode and the ability to link existing projects to syncs — `dadf87f93d9214dcd741c7ed503dfbf504c9df54` (#3932) (`kmendell`)
- Added configurable Git identity for pushes — `372f168940702645d63d34d0d6d80b742a7cb8da` (#3935) (`kmendell`)
- Added an explicit environment-backup switch with a warning for project backups — `9ba395d5272ee5a3506682e61b9d1e06c4752f16` (#3937) (`kmendell`)
- Added support for binary files in Git backups and reused the editor file tree in the sync dialog — `b80880fab4146572c02f071126d44751391031b9` (#3946) (`kmendell`)
- Preserved pre-deploy hooks and deployment options when importing GitOps syncs — `da22a65fac881f7b6ff6048bae91399608d2eeab` (#3952) (`kmendell`)
- Added S3 backup discovery and re-keyed volume backups to the recovery key — `02208f5cd24e028dfb0e2221305febbf3ee58837` (#3885) (`neurekadev`)

## Security and vulnerability scanning

- Added CSV export to the security vulnerabilities table — `af5538dde929bcc0e94053fd9ad4f3d263820da7` (#3936) (`kmendell`)
- Added a **Fix available** filter to the security vulnerabilities table — `bf48ce6eebebca249607b02dcebb7df37079ab90` (#3944) (`kmendell`)
- Added vulnerability-count sorting to the images table — `9f89ceefbfcd6dd16581cf520b40d4b201d41801` (#3945) (`kmendell`)
- Added Trivy configuration and ignore-file settings for vulnerability scans — `eff16eb5688e981b36847f55995d92990d02d30c` (#3949) (`kmendell`)
- Added support for decimal CPU limits in Trivy scans — `02225a2d03529ef14ec7e4009f6f4b746feca2b6` (#3972) (`kmendell`)

## Containers and networks

- Added subnet and gateway columns to the networks table — `146b4b9658d163c1290453203172b3cd286bdf3f` (#3942) (`kmendell`)
- Added IPv6 addresses for containers and network members — `8345e5669ac2580a0b67385fbb9868ab6468e97e` (#3957) (`kmendell`)
- Added optional IP ranges when creating networks — `38d3ed8c19faea20e4d9457395519196f074ce6a` (#3959) (`kmendell`)
- Linked container overview volume and network counts to their respective tabs — `f9988528d1a94e3440d2d295bfcd1c658aff1798` (#3958) (`kmendell`)
- Added support for downloading container logs — `9a83c5ac02944dc72c3fc4a39691eb8a1a832403` (#3960) (`kmendell`)

## Interface and settings

- Added Shift-select range support to selectable tables — `472b01f5855c642afe192be448fe6339f6a6f74c` (#3943) (`kmendell`)
- Clarified ntfy token authentication, required Matrix credentials, and cleaned up test menu labels — `e68ab5ab4b8e3df7528b6f365846c383a49d381e` (#3951) (`kmendell`)
- Updated theme colors and added support for inheriting theme colors — `eee8a84bd5d467ca5f1f6dac4ca20768a377dcf4` (#3965) (`kmendell`)

## Authentication, permissions, and reliability

- Surfaced OIDC and settings errors, allowed clearing the OIDC secret, and required it while OIDC is enabled — `70cf8a2596d32f2fd84685e5827bc79f95e2a3b7` (#3954) (`kmendell`)
- Hid the Automations tab and stopped job polling without `jobs:manage` permission — `94d6d79d533cd941d46d2cacaba391167d570c96` (#3963) (`kmendell`)
- Updated the legacy `users.roles` backfill to run only once — `a4ae1a0f20463c7dc018f7bae4996e3cab2e34ac` (#3964) (`kmendell`)

## Agents and edge tunnel

- Fixed Docker snippets to use the port number from the agent address input — `2c34ade39db5f8686f73e7f76bb8df691f39cebc` (#3955) (`tinkrynite`)
- Added missing job commands to the edge tunnel — `6af4589a12376f7fe40a9f1d8778fbbe6e5afbf4` (`kmendell`)

---


## 2.11.1

_2026-09-14_

> [!NOTE]
> This release fixes authentication, vulnerability-management, and Docker compatibility issues.
> It also improves file-mode preservation, project navigation, notifications, and dashboard reliability.

## Authentication and notifications

- Align first-login password validation with the configured policy — `b71892d0902480a5e430bf4c10439d167508301c` (`kmendell`)
- Prevent passkey MFA failures caused by reactive options — `95e5a3bc14706e69a9a366794d92706c55a53a7f` (#3926) (`kmendell`)
- Preserve notification event subscriptions during partial settings updates and warn when none are subscribed — `817444abbd7984401e9f2e3ef59447e474f8d97b` (#3925) (`kmendell`)

## Vulnerability management and Docker compatibility

- Fall back to `cpuset` for Trivy when the Docker host lacks CFS quota support — `41eae6334d8807d7fd7c159b73f157b28422e2a8` (#3910) (`kmendell`)
- Allow vulnerability management to be disabled per environment — `4e432b30a69458c2077f9eac5ceb1a83709282b6` (#3914) (`kmendell`)
- Pull pre-deploy runner images using configured registry credentials — `989e160ec544842d0db69ade182037cd2976b310` (#3918) (`kmendell`)
- Fail image patching early when Docker lacks the containerd image store — `6fdcf37c2dba690a227d09e1bf5aaf3a3c632330` (#3928) (`kmendell`)

## Projects and GitOps

- Preserve existing file modes when saving project and global environment files — `c92f3932cca1597c3d3fe3484b3d9a1e41289216` (#3919) (`kmendell`)
- Preserve configured directory modes for GitOps directory-sync project roots — `31081659de472c06a639bd53e1615dff5077b5ac` (#3920) (`kmendell`)
- Treat identity bind mounts as mapped when re-resolving escaped relative Compose paths — `7b19b854ace736bedafd0a632e8cb54f3420696a` (#3924) (`kmendell`)
- Sort projects by live status instead of the stale persisted status — `0b594d1fd17c50285d9e7956f7bc8a488200c27b` (#3923) (`kmendell`)

## Interface and dashboard

- Keep the environment detail page stable when navigating back to the list — `73d478211ebe090dc5afe38c098361e2b5770472` (#3921) (`kmendell`)
- Prevent the project log panel from recursing on the viewer stop callback — `e8cc329c1042b896669ae091d64e6a8cbd15bf0f` (#3927) (`kmendell`)
- End dashboard statistics loading on stream failure and retry on refresh — `16b7b59d5e6442bd8d2e0181356fb8f5d6488ebc` (#3930) (`kmendell`)

---


## 2.11.0

_2026-09-13_

> [!NOTE]
> This release adds durable job execution, Docker event streaming, label-based container management, non-UTF-8 workspace uploads, and improved backup recovery. It also improves synchronization, auto-update recovery, project workflows, frontend performance, and error reporting.

### Features and improvements

- Added durable job execution with offline recovery (`58823c2ac4afd674f88bccee38b5da9e985da335`, #3863) (`kmendell`).
- Streamed Docker daemon events to the Arcane event log (`2a61718a3af98393ddcdef40d02d30fe416c70ec`, #3862) (`kmendell`).
- Added tag-based container updates (`f8f456a62b866108b3c8c433cb04f8d6f6c119bb`, #3865) (`kmendell`).
- Added filtering for containers and projects by labels, plus support for hiding containers through labels and Compose metadata (`5dfd281c628bac0b4880c465bd629040d94a301e`, #3874; `6006dbefd64e437e4935c80e38006b96d53c4b93`, #3875) (`kmendell`).
- Added support for uploading non-UTF-8 workspace files (`a38a0a2fbff09efeeda6430be15c933af00719fe`, #3859) (`neurekadev`).
- Linked Git-managed projects and synced files to their repositories (`6eed748e3fc976ebeaa8a227239ec8b7565ad86d`, #3879) (`FusselTV`).
- Added backup recovery key import and reset confirmation controls (`16db5a33747b49350407073aa0040baa5e151947`, #3884) (`neurekadev`).
- Improved environment synchronization by surfacing failures and recording manual sync activity (`c6ad1016d913efd100260967dc1a5da8cacb7e32`, #3854) (`kmendell`).
- Improved auto-update recovery, job activity integration, and per-container outcome reporting (`5d055f66d796ca79aabf59adfd3d4b7c3986404e`, #3867; `472b50ed15b7d550eee0173e01f1f77a129601bb`, #3872) (`kmendell`).
- Improved user validation and password policy feedback (`832cf6eca30be8fbf5f3eb19786f5d8a16c43751`, #3851) (`baejihoon`).
- Improved frontend table scrolling while preserving glass effects (`ca9917fc5cbdd462734754409c5657908bd30475`) (`kmendell`).
- Improved Easy Join discovery on non-manager environments (`436e20bcee37138fc369eecf02eab6b5f682ace2`, #3902) (`kmendell`).
- Improved diagnostics for projects with unreadable `.env` files (`e8c92c5e6bae99251268749c4d16c53ecbebd16f`, #3909) (`kmendell`).

### Fixes

- Surfaced JWKS initialization errors instead of readiness timeouts (`eeb79a68e86702825eca2531b933c9bd801d6201`) (`kmendell`).
- Normalized identity and display text with Kit struct tags (`b51613f2c6d389e20be507be5db1f53f48600ac6`, #3856) (`kmendell`).
- Detected missing S3 repositories and paused remote backups (`4ff83971e1a20c80ceceba27153549047461898d`, #3869) (`kmendell`).
- Enforced administrator requirements for environment tokens (`4fe4c7ed9483dff7d43e3c4bee6a92037ce886b4`, #3881) (`kmendell`).
- Added missing edge tunnel routes (`88514f756319179e407c96a5b9226825944ccdc0`) (`kmendell`).
- Clarified credential requirements when changing repository URLs (`2081406e035b0f761533fa23fdae5e5c810ecdb7`, #3886) (`kmendell`).
- Fixed mobile navigation label stacking (`de66d6330d6cf19584b38aa201ae56f8047ebda1`) (`kmendell`).

### Refactoring

- Centralized Compose workflows and consolidated project files (`9b45fd5572e59b0696ebfcb61a7d7a983ea0b4ce`, #3860) (`kmendell`).
- Consolidated frontend components (`1ad4f285c5a2b8ac471c9fa1b1100b1ab773d5fa`, #3864) (`kmendell`).
- Adopted Echo request IDs and SPA static middleware (`f8e4f75473950f4772bb141a8554ce22d9d99393`, #3873) (`kmendell`).
- Replaced `svelte-check-rs` with the official `svelte-check` (`3dac10c2d53e2cae90e63b18e468c9263347c693`) (`kmendell`).
- Cleaned up effect usage and adopted proper Svelte reactivity (`5df09ed475c4bac4ab6f54e1b9bdde1ac1c55105`, #3904) (`kmendell`).
- Aligned frontend stores and attachments with Svelte best practices (`2d0ac4a8c93c04d5657a7c3c5880b739703bc8cd`, #3907) (`kmendell`).

### Dependencies

- Updated `go.getarcane.app/acfs` to v0.6.0 (`5ba5ad4d20d8302051112c9c18ad29edc90c2d9f`, #3857) (`kmendell`).
- Updated `charm.land/log/v2` from 2.0.0 to 2.0.1 (`57bd14795309fe11901cef510dc1cf8332c9a984`, #3890) (`dependabot`[bot]).
- Updated `github.com/containerd/containerd/v2` from 2.3.4 to 2.3.5 (`a8bf2eee2bf1f197cd254b356e89d80c3df08e3f`, #3895) (`dependabot`[bot]).
- Updated `golang.org/x/time` from 0.15.0 to 0.16.0 (`34e73a4648f34205a86f3bb460b0626307d72c8e`, #3894) (`dependabot`[bot]).
- Updated `golang.org/x/sys` from 0.47.0 to 0.48.0 (`987548b2a807d8d0916592faab55c589abb74a68`, #3889) (`dependabot`[bot]).
- Updated `golang.org/x/oauth2` from 0.36.0 to 0.37.0 (`6a9ff7aa64fbec74e379b5dc9622699189093d73`, #3892) (`dependabot`[bot]).
- Updated `golang.org/x/sync` from 0.22.0 to 0.23.0 (`a50ab984528cf443c07e1a58052a09af2632dd63`, #3893) (`dependabot`[bot]).
- Updated `golang.org/x/mod` from 0.40.0 to 0.41.0 (`a423806a3f1c99073382c6e3b0ce5184aecdd52b`, #3888) (`dependabot`[bot]).
- Updated `modernc.org/sqlite` from 1.57.0 to 1.58.0 (`da6712ddfee61340f5286aa9982fd5f8f87bc647`, #3891) (`dependabot`[bot]).

---


## 2.10.2

_2026-09-05_

> [!NOTE]
> This release adds performance improvements across authentication, Docker calls, scheduled scans, project loading, and CVE serving. It also fixes container shell handling, scheduler overlap, image digest display, authentication flows, agent reconnection, and several UI and integration issues.

## Fixes

- Surface container shell close reasons and disable WebSocket compression — `30e1bb590a34d9aeded744a71d39e8709b3b7b33`, `b9b8c813f6d3735770f2bc1bf5c80b4967eb7e26` / #3814 (`kmendell`)
- Skip overlapping scheduled cron job runs — `51cf6bb6a1049a01596a5933ddb2508de6938dea`, `5232b4d04bfb482ed9d3ec869f30ec2fee03ca30` / #3815 (`kmendell`)
- Refactor image digest inspection logic — `d256fc2787efd18341d84e63e59e7cd38300e99d` / #3826 (`jdrouhard`)
- Keep the first-login password dialog open and clear its flag after an admin password change — `6f90da73f3606e42e58a0f00c98208f5817c27c9` / #3831 (`kmendell`)
- Allow the frontend to recover from stale chunks after a redeploy — `8c294a7316f3616a704edfec366f4adccd91a577` (`kmendell`)
- Keep edge agents on gRPC after a manager restart — `4febf6d0dcf64997b53cba7743d082b4c7550979` (`kmendell`)
- Show digest-pinned image references — `5c0592994278ac9ab4600966dcdb460ecbef857b` / #3836 (`kmendell`)
- Hide passkey login when no credentials are registered — `b7b568e1a6911bf1b8369de129af792fb0f23e4e` (`kmendell`)
- Honor SSH usernames in Git repository URLs — `837fe10c92ddbb2391d7b13475d475a68f7fca2c` (`kmendell`)
- Support host memory accounting for Docker inside LXC — `416e93a4be282a5d26025f9fd00f3ae56c70bb71` / #3846 (`kmendell`)
- Load project digests automatically when entering the Updates page — `8d5eaeb9dda0072736eb67272de4d7d781d7472b` (`kmendell`)
- Sort environments by name while placing the current selection first — `4e0f1dcfc074a4562b59ef03ee49dabbd68f40f5` (`kmendell`)
- Rate-limit webhook triggers by token instead of client IP — `de1e978810a490fd7930a634bd4df765a50f65b2` / #3742 (`ohOgil`)
- Run backups in an activity instead of locking the UI — `dd16fd942afaed4db4d63ad4fa787bce20251f8d` / #3847 (`kmendell`)

## Performance

- Serve cached authentication token state — `65fcfa637919992c81b373396265455a1f08ae3c`, `5e8ecabd4f0c2aa3c3011baab6761dc13aabc96f` / #3816 (`kmendell`)
- Route Docker daemon calls through singleflight callers — `64c2500b669f343451f2541a045b578e10ee172b` / #3818 (`kmendell`)
- Share one stream producer across all clients — `db9878d01dccfed0406cee7c5e573a7f2c8637fe` / #3819 (`kmendell`)
- Look up Copa targets by digest instead of issuing database queries — `ddcd50530759ed0b31d2f717e7d82a213bfd655d` / #3821 (`kmendell`)
- Validate the projects metadata cache by file modification time and resolve entries in parallel — `0b44e8dd5e16131015e18c70601c79b1c2cad9a9` / #3823 (`kmendell`)
- Serve the CVE list from a normalized table — `31a9896dfb052ece34d7ba4c14d8347b9ad8a8b5` / #3825 (`kmendell`)
- Bound scheduled scans, skip unchanged images, and avoid remounting during navigation — `8299a56c5430f9a04ee132b15f58a9e46f810dc0` / #3829 (`kmendell`)
- Dispatch notification providers concurrently — `767992dc34a4b98d35fb220425bf51ea9b123961` / #3820 (`kmendell`)

## Improvements and refactoring

- Allow selecting registries for Arcane Tools and Trivy databases — `b85caceeef0ba5dd094ce1fcb82805acfd2a6445` / #3830 (`kmendell`)
- Clean up duplicate HTTP client logic — `8a74c1b729e6bd0b02e1bcf17a824cc445cb2eb3` (`kmendell`)

## Dependency updates

- Bump `react-email` from 6.9.2 to 6.9.3 — `0cbb2590792d935ff2c7a20468e91edc3b318304` (`dependabot`[bot]); `8dd0210aec7303fa506cdf77e235804961107a7d` / #3806 (`kmendell`)
- Update the `tanstack-table` group with two updates — `f58bc47404cbc61ca0c2a62a4d48436e5b2b5688` (`dependabot`[bot]); `0a7ea50401c95aff42f90b94172b0c2f42ece4cb` / #3799 (`kmendell`)
- Bump `ky` from 2.0.2 to 2.1.0 — `5700fb7f52a95e3e21ab0fce8d5a7e453cb0201c` (`dependabot`[bot]); `ab3eaae5751c0abe8ae326b35b565b43f2ed678f` / #3802 (`kmendell`)
- Bump `svelte` from 5.56.10 to 5.57.0 — `3db4f4a591fd43aff54d5d9ca679780f6d9c5e35` (`dependabot`[bot]); `efb72f212b9795ecccfcf3b792a0e76dedebb1c6` / #3808 (`kmendell`)
- Bump ``tanstack`/svelte-query` from 6.1.43 to 6.1.48 — `917e6c267748da315f93855dadbea3444466f12c` (`dependabot`[bot]); `242744189b8e1065be494d5cd25c8a97ecb511c0` / #3807 (`kmendell`)
- Bump `pnpm` to v12.2.1 — `7d33837fd5a2d8bfd9617dabe7347f1ea06ca1c2` (`kmendell`)
- Bump ``xyflow`/svelte` from 1.6.3 to 1.6.5 — `1829e66194ca404bd8ff4b211e693a7d149ac184` / #3810 (`dependabot`[bot])
- Bump `github.com/nicholas-fedor/shoutrrr` from 0.18.0 to 0.19.0 in `/backend` — `343dc2020f10c610f81bb5834bcc5b858377afbe` / #3840 (`dependabot`[bot])
- Bump `golang.org/x/crypto` from 0.55.0 to 0.56.0 in `/backend` — `37226ae35749563489551c840ca2b1620e07e3e6` / #3844 (`dependabot`[bot])
- Bump `github.com/coreos/go-oidc/v3` from 3.20.0 to 3.21.0 in `/backend` — `504e5b6dda7ea16bdab84eb6b72c1da47e90414d` / #3845 (`dependabot`[bot])
- Bump `github.com/pressly/goose/v3` from 3.27.3 to 3.28.0 in `/backend` — `d6271ea3045682a063b516ea0f1d71fc2f7e4a64` / #3841 (`dependabot`[bot])
- Bump `github.com/klauspost/compress` from 1.19.2 to 1.20.0 in `/backend` — `71d866fefdd79552afd4db123fd01cbd29199753` / #3843 (`dependabot`[bot])
- Bump `github.com/mattn/go-runewidth` from 0.0.28 to 0.0.29 in `/cli` — `5137b5dcb4efe2e6107adc7d316af4ec9132f01b` / #3838 (`dependabot`[bot])

---


## 2.10.1

_2026-09-02_

> [!NOTE]
> This release fixes browser session cookie size issues and missing sidebar usernames.
> It also updates the gopsutil and zod dependencies.

### Fixes

- Issue compact browser session cookies instead of the larger ML-DSA access JWT — `df7c24c85e9d4fa9d380532db928ecc3aa13dc17` (`kmendell`); `e874390f6b4ca611a4b5f733868cca1e01138aca` / #3813 (`kmendell`).
- Fall back to the username when the display name is unset in the sidebar user menu — `0c7174f1089079d79535563ac2d54b032ea6914a` (`kmendell`).

### Dependencies

- Bump `github.com/shirou/gopsutil/v4`, including 4.26.7 to 4.26.8 — `c9ad2e6f9f9751764e5608128b7e55abf9349b88` (`dependabot`[bot]); `dd02c011c575c298dbd3f3167e4ba291edc06c59` / #3798 (`kmendell`).
- Bump `zod` from 4.4.3 to 4.5.4 — `d95764cf15a2867e2ac573f8cf6f78654da597d7` / #3812 (`dependabot`[bot]).

---


## 2.10.0

_2026-09-01_

> [!NOTE]
> This release adds selective system restores, scheduled volume backups, Convert to Compose, direct image patching, Apple push notifications, and ML-DSA-87 signing for authentication and edge mTLS.
> It also improves upgrade reliability, image update discovery, project logs, Swarm access, backup downloads, and UI performance.

## Backups and recovery

- Add selective system restores and harden recovery — `874134cfedc2e8ae44e6f0cd7e957e87bc760988` (#3708) (`neurekadev`)
- Add system-managed volume backup scheduling — `53a959f93e26193c6c9ac4bb4053555f18e8b2ff` (#3737) (`neurekadev`)
- Reduce S3 backup download amplification — `538350af6c72d0e6d80164f6aacb3b8dd02cd1bd` (#3773) (`kmendell`)

## Containers, images, and projects

- Add experimental Convert to Compose for running containers — `9aef34e4ea703cbee29d31a4338413018ce54f34` (#3746) (`kmendell`)
- Add direct Copacetic image patching — `3617720518f0bdfc4282a7b5844508f3b7da60ec` (#3744) (`kmendell`)
- Restrict Copa patches to OS-level issues — `e95c55326a59a615993113e0ee809df734a40b0a` (`kmendell`)
- Store raw Trivy reports in the database and fix patch-target pagination — `53e7e82715bfebd9ed0d1a27dc0b777d0bbd68b0` (`kmendell`)
- Honor UI container exclusions during image update discovery — `b9b2092c0e36d3eaba66335e003b7ecd5e858540` (#3763) (`kmendell`)
- Stop checking Arcane-built local images against registries — `8d10b7db2d34aefa44f0f9a684f3b84b2ae355d7` (`kmendell`)
- Query image update information directly instead of caching it per container listing — `509c6e81babef8c3ac0b4bf9c418c1d3115c50a8` (`kmendell`)
- Resolve upgrade target images for blank-target self-upgrades — `8bbd157a2963874998c481487d132f460cb15b1c` (#3772) (`kmendell`)
- Honor `COMPOSE_FILE` and other predefined environment variables in projects — `6e1dbb2d22858014970f3b235b73acea0fc70ad5` (#3709) (`kmendell`)
- Forward registry authentication when deploying Swarm stacks from Git Sync or source edits — `1570a1da7a00e20012dcdf3d5496e2d9f39f732f` (#3787) (`elfensky`)

## Reliability and operations

- Return HTTP 200 from the environment health check — `6b46c36a237328fc0728ac8fd02e8bc79a149986` (`kmendell`)
- Write workspace files using the volume’s runtime identity instead of root — `f4958ae8c8318bd921ef5be4ae9a6436309b0cd5` (#3745) (`kmendell`)
- Match Docker CLI memory accounting for cgroup v1 containers — `4c479f1c5b01ae6932e2de2ad91ca79aa6874edc` (`kmendell`)
- Purge leftover Git clone scratch directories — `d57d18f1136c1e518be656615f6e2df01d777e34` (#3771) (`kmendell`)
- Preserve stderr streams and Docker timestamps in project logs — `cac8a9090baea3b44aa2868d195cd21c3dcf62f0` (#3770) (`kmendell`)
- Allow the Viewer role to browse Swarm resources — `3f205dea94fa6d40189cf80eec665e39c5131e60` (#3779) (`kmendell`)
- Trigger agent self-upgrades asynchronously and pass the manager’s resolved target version — `d857910d9b08f96e7739c679415b18fd192a01b4` (#3786) (`kmendell`)
- Serialize explicit empty values in partial-update DTOs with `omitzero` — `24592561d9154da35d7fecd380e12c50e3c5783a` (`kmendell`)

## Authentication and notifications

- Add native Apple push notifications for the iOS app — `4b1abefec6b9b93ea331e1259e061c6862025a11` (#3783) (`kmendell`)
- Sign and verify sessions, OIDC, passkeys, and edge mTLS with ML-DSA-87 — `2993fd316d41fafc110476370870a49b9202969c` (#3785) (`kmendell`)
- Accept display names in email notification From addresses — `034e7e7efd4e756f0c3145099f24f2afe0bc7596` (#3776) (`ohOgil`)

## User interface

- Improve table-scrolling performance across all views — `1243d8e0bafdc110b8443ef710d999e5104ce632` (`kmendell`)
- Keep dialog widths within the content area — `5e7acb61d3bb6ef4bc60a80266c424f1d40d02c2` (`kmendell`)
- Move frontend files into a more maintainable structure — `938882a174322a1bbc6fe50fe926a4c61820797b` (`kmendell`)

## Dependencies

- Bump `github.com/aquasecurity/trivy` from 0.69.3 to 0.72.0 in `/backend` — `29784fead298d740fb07db2754d53ff55e3bb9c4` (#3748) (`dependabot`[bot])
- Bump `github.com/sirupsen/logrus` from 1.10.0 to 1.10.1 in `/backend` — `be4220fa2cc44c348177de5c39354b716a517413` (#3753) (`dependabot`[bot])
- Bump `github.com/quay/claircore` from 1.5.52 to 1.5.53 in `/backend` — `611b5d7ff298ccd51b0675874354282aa117356c` (#3747) (`dependabot`[bot])
- Bump `github.com/samber/hot` from 0.13.0 to 0.13.1 in `/backend` — `cecdb777ec0f3487ab442f2bd1a45dc1120f0564` (#3754) (`dependabot`[bot])
- Bump `charm.land/bubbles/v2` from 2.2.0 to 2.2.1 in `/cli` — `b631dd67c34ef642c3bb177ad3d9df09c0974b0d` (#3752) (`dependabot`[bot])
- Bump ``tanstack`/svelte-query` from 6.1.39 to 6.1.43 — `55762896904d9453f2c2cbeb26645b6e0f6818a2` (#3761) (`dependabot`[bot])
- Bump `github.com/google/go-containerregistry` from 0.21.9 to 0.22.0 in `/backend` — `3c0aac7c557ace8a15ffbd60dd6dc4448ec43a93` (#3755) (`dependabot`[bot])
- Bump `marked` from 18.0.10 to 18.0.11 — `201a3ff183a0f07e342398d0b1b12ca3847777a5` (#3759) (`dependabot`[bot])

## Refactoring

- Use generics to eliminate redundant logic — `671ebef57234c40acba287925c5ee6ac9efdd399` (#3683) (`kmendell`)
- Migrate JWT and JWKS handling to jwx v4 — `45b063a074f521f45591f58c7b1d89b4395dc267` (#3790) (`kmendell`)

---


## 2.9.0

_2026-08-26_

> [!NOTE]
> This release adds automated S3 backups, standalone container editing, batched container-update notifications, and GitOps redeployment for stopped projects.
> It also expands the CLI with commands for vulnerabilities, activities, and webhooks while improving project handling, OIDC support, and API behavior.

## Features

- Add automated S3 backups — `555c5dfbac478e7bbe336fff29cfe4d6277b2d6d` (#3459) (`affeldt28`)
- Batch container-update notifications — `0239f62b45c7b2d485285a54fc54e44b6a0721e8` (#3650) (`wyx1818`)
- Add the ability to edit standalone containers — `24894fc7310b747e95d2b5d59e0a5100bb8bf12f` (#3646) (`kmendell`)
- Add renaming for unused volumes — `1e413ada2f2413fa2a7c97798288b73ddb9d6b6c` (#3704) (`neurekadev`)
- Pull and redeploy images after GitOps sync for stopped projects — `f17e84412ae41dac1945f5b6896bafce8c244bb9` (#3698) (`ohOgil`)
- Add CLI commands for the latest server features and adopt hyphen-free command naming — `d84511b8d67bef5fe3117832c9173ecc987e38da` (`kmendell`)
- Add CLI commands for vulnerabilities, activities, and webhooks with end-to-end coverage — `f4f9df0e8f1373a9e7fcba145383fcda8b4b3fc2` (`kmendell`)

## Fixes

- Ensure stable ordering for page walks without an explicit sort — `bf783a828ef816acf789851dffb2b8b8ed249450` (#3648) (`kmendell`)
- Percent-decode API path parameters before use — `c469305cdacf49b80f7f5639f00eef597c1ca102` (#3682) (`kmendell`)
- Show Git-synced project files as read-only instead of hiding them or crashing — `da21e2694754434d9b6b936834aed44173378ef1` (#3690) (`kmendell`)
- Send the build directive for projects from the backend — `53017c67cf91656965d667bf23e80dc0b46bd482` (#3691) (`kmendell`)
- Allow assigning roles to OIDC users and handle username collisions during OIDC login — `c38c2580fb14f8a8f15f74ef9203bfdb9896c466` (#3692) (`kmendell`)
- Make the update-all dialog scroll correctly with large fleets — `b7a619b2c7f8f08f05cc78f3db51ac6aa63e9ef4` (`kmendell`)
- Validate cgroup-derived container IDs during self-detection with `network_mode: service` — `c24013107eff0adba0813b66b50350a03a487895` (#3710) (`kmendell`)
- Handle updates to discovered Compose projects — `275aface140f0d41e67adef402859f8a2a6ba207` (#3711) (`kmendell`)

## Refactoring

- Upgrade to Go 1.27.0 — `b22edb6909c5497f3e72e27d4570551c7fb2fe91` (#3680) (`kmendell`)
- Use native Temporal for time and date parsing — `60c2dee13cd9405175b6e0a5d65053724bb005cd` (#3714) (`kmendell`)
- Use AI generation for GitHub release notes — `2240e8f0c7a49331f3ba7e385d7b793b844fbcaa` (#3739) (`kmendell`)

## Dependency updates

- Bump ``sveltejs`/kit` from 3.0.0-next.21 to 3.0.0-next.23, then to 3.0.0-next.25 — `cd8c48b36c926a5342a5b83f373269e4ec4093a1` (#3672), `70475a2cf7ca52f2d017422551ce645e4507aaa4` (#3727) (`dependabot`[bot])
- Bump `svelte` from 5.56.8 to 5.56.9, then to 5.56.10 — `d4d83941e2685a2273a6437670d17b71816d5620` (#3666), `1478dd7df710c42161b16f7cb07be79193e7570b` (#3724) (`dependabot`[bot])
- Bump `svelte-sonner` from 1.1.1 to 1.2.1 — `39ce639ebe506fb4b825fd08335a98bc55a7efd1` (#3670) (`dependabot`[bot])
- Bump ``xyflow`/svelte` from 1.6.2 to 1.6.3 — `a59e1e8cdc38513921dc01c4e686581e208fd10d` (#3668) (`dependabot`[bot])
- Bump `charm.land/lipgloss/v2` from 2.0.5 to 2.0.6 in the CLI — `98f5cac2359a4b5dd42355b9c4da24ebc1a1739d` (#3662) (`dependabot`[bot])
- Bump ``codemirror`/view` from 6.43.8 to 6.43.9 — `f2f34fac879a920afa772c0a6f0b509b7a74c478` (#3664) (`dependabot`[bot])
- Bump the AWS SDK for Go v2 dependency group with five updates in the backend — `ffc1f128f487d1942f56c636c1daeba757649e57` (#3731) (`dependabot`[bot])
- Bump `github.com/samber/slog-echo/v2` from 2.0.0 to 2.1.0 in the backend — `6c3f58f2b29acfb341d2071857e089b68f69529d` (#3735) (`dependabot`[bot])
- Bump `go.getarcane.app/acfs` from 0.4.1 to 0.4.2 in the CLI and backend — `65b97153f05e20107ca04c70c60f3c1210409ac3` (#3722), `8087a444b343b9e8099df3a40d3a6fbb67c85441` (#3732) (`dependabot`[bot])
- Bump `github.com/mattn/go-runewidth` from 0.0.27 to 0.0.28 in the CLI — `3f43202a9c7550d087f60077a53ae59616b925f7` (#3719) (`dependabot`[bot])
- Bump `go.getarcane.app/updater` from 0.7.2 to 0.7.3 in the backend — `d9b58268878a9a3de48013a6aa6d1f2b1681112b` (#3733) (`dependabot`[bot])
- Bump `go.getarcane.app/docker/convert` from 0.1.0 to 0.2.0 in the backend — `8d52e7a704f675fcc01a31a72b29b9a8e5799011` (#3734) (`dependabot`[bot])
- Bump `charm.land/bubbletea/v2` from 2.0.8 to 2.0.9 in the CLI — `f012dbf16a2473a78fa9c44e6b2e7c384e522679` (#3720) (`dependabot`[bot])
- Bump `charm.land/bubbles/v2` from 2.1.1 to 2.2.0 in the CLI — `29c5dc6afd682d08a26185ec5aa07a88e17ac6f9` (#3721) (`dependabot`[bot])
- Bump ``tanstack`/svelte-query` from 6.1.38 to 6.1.39 — `d200e9f73555d23b26a7e80270283fa3a9302213` (#3725) (`dependabot`[bot])
- Bump `bits-ui` from 2.18.1 to 2.19.0 — `3646524258861b2f6d67f7bd67be4b96a830e0c0` (#3728) (`dependabot`[bot])
- Bump `marked` from 18.0.9 to 18.0.10 — `82a2771040785fd68d44d1c66d8691543b3c4d7a` (#3729) (`dependabot`[bot])
- Bump `vite-plus` to 0.3.0 — `f3002cbd30db67f0bd4e05feaf9dec830b1b35a3` (`kmendell`)
- Bump ``tanstack`/virtual-core` from 3.17.7 to 3.17.8 — `ea1ab4b086e857166bdff9f43fabb92f7cdb7360` (#3723) (`dependabot`[bot])

---


## 2.8.1

_2026-08-21_


### Bug fixes

* report never-pulled image refs as a distinct 'not pulled' state instead of failing the update check ([#3631](https://github.com/getarcaneapp/arcane/pull/3631) by `kmendell`)
* localize category cards ([#3596](https://github.com/getarcaneapp/arcane/pull/3596) by `InfinityPacer`)
* coalesce concurrent Docker image/container list calls to cut duplicate decodes ([#3635](https://github.com/getarcaneapp/arcane/pull/3635) by `kmendell`)
* gate project archiving on live Docker state instead of stale persisted status([c487936](https://github.com/getarcaneapp/arcane/commit/c487936819c6dfbc8ed651293f149d89bf76edfd) by `kmendell`)
* use stored credentials for non-Docker Hub registries ([#3639](https://github.com/getarcaneapp/arcane/pull/3639) by `BobzTH`)
* add missing options to project redeploy dropdown([f7cb885](https://github.com/getarcaneapp/arcane/commit/f7cb8856213112e994132dbf8380bfca91dd4ecf) by `kmendell`)
* go1.26.6 h2c ReadHeaderTimeout regression([6d4f222](https://github.com/getarcaneapp/arcane/commit/6d4f222389cfa48e52d575bc3ae4a459904053cc) by `kmendell`)
* use errors.Is(err, fs.ErrNotExist) for acfs error checks ([#3647](https://github.com/getarcaneapp/arcane/pull/3647) by `rohitkumbhar`)
* unblock git sync workspaces and pre-deploy hooks on permission edges ([#3637](https://github.com/getarcaneapp/arcane/pull/3637) by `kmendell`)

### Dependencies

* bump the aws-sdk-go-v2 group in /backend with 3 updates ([#3616](https://github.com/getarcaneapp/arcane/pull/3616) by `dependabot`[bot])
* bump golang.org/x/mod from 0.38.0 to 0.39.0 in /backend ([#3618](https://github.com/getarcaneapp/arcane/pull/3618) by `dependabot`[bot])
* bump the tanstack-table group across 1 directory with 2 updates ([#3613](https://github.com/getarcaneapp/arcane/pull/3613) by `dependabot`[bot])
* bump github.com/docker/compose to v5.5.0 ([#3643](https://github.com/getarcaneapp/arcane/pull/3643) by `kmendell`)
* bump to go v1.26.6([ce37184](https://github.com/getarcaneapp/arcane/commit/ce371847f03e061e4adabdbaf36b52933512b9cb) by `kmendell`)

### Other

* move models into standalone domain packages ([#3636](https://github.com/getarcaneapp/arcane/pull/3636) by `kmendell`)



**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v2.8.0...v2.8.1

---


## 2.6.0

_2026-07-30_


### New features

* raw docker CLI output for all operations + interactive watch mode ([#3376](https://github.com/getarcaneapp/arcane/pull/3376) by `kmendell`)
* clickable dashboard tiles, volumes tile, and default landing page ([#3383](https://github.com/getarcaneapp/arcane/pull/3383) by `kmendell`)
* add row, bulk, and Update All actions to the updates page ([#3398](https://github.com/getarcaneapp/arcane/pull/3398) by `kmendell`)

### Bug fixes

* harden gRPC tunnel reliability and request lifecycle ([#3325](https://github.com/getarcaneapp/arcane/pull/3325) by `kmendell`)
* prevent image update checks from getting stuck in a running state ([#3327](https://github.com/getarcaneapp/arcane/pull/3327) by `kmendell`)
* preserve IPAM fields in network inspect responses ([#3335](https://github.com/getarcaneapp/arcane/pull/3335) by `kmendell`)
* remove full stack trace from logging([1bed071](https://github.com/getarcaneapp/arcane/commit/1bed0714fc36f9bd0d0c170f0fcedd1bfe33e380) by `kmendell`)
* cor

---


## 2.5.0

_2026-07-21_


> _Maintenance (2026-07-25):_ **Fix the app being SIGKILLed instead of shut down cleanly.** The AppArmor profile granted `signal (send) set=(...)` but not `receive`. AppArmor mediates signal *delivery* as well as sending, so s6's SIGTERM never reached the app: the graceful shutdown was skipped and the container was killed after the grace period (exit 137). The rule is now `signal (send,receive),`. Measured on one identical image with only this rule varied: no signal rule -> 13.2s/exit 137; `signal (send) set=(...)` -> 12.7s/exit 137 with no cleanup logged; `signal (send,receive),` -> 6.9s/exit 0 with the full s6 shutdown sequence. Now enforced by `.github/scripts/validate-apparmor.sh` (rule 4) and by the smoke test, which fails on exit 137 under confinement. Rebuild/reinstall the app to pick up the corrected profile.

### New features

* unify swarm node agent deployment and easy join swarm agent/nodes ([#3279](https://github.com/getarcaneapp/arcane/pull/3279) by `kmendell`)
* replace scheduled image polling with Docker event-driven update checks ([#3290](https://github.com/getarcaneapp/arcane/pull/3290) by `kmendell`)
* redesigned events view to remove bulky dialog ([#3301](https://github.com/getarcaneapp/arcane/pull/3301) by `kmendell`)
* redesigned templates browser and view ([#3304](https://github.com/getarcaneapp/arcane/pull/3304) by `kmendell`)
* redesign global variables with environment scoping and secrets ([#3311](https://github.com/getarcaneapp/arcane/pull/3311) by `kmendell`)
* rework activity center ui/ux and backend lifecycle ([#3313](https://github.com/getarcaneapp/arcane/pull/3313) by `kmendell`)

### Bug fixes

* preserve tag when saving update records for tag`digest-pinned` images ([#3242](https://github.com/getarcaneapp/arcane/pull/3242) by `pkoutsovasilis`)
* tab routing for all pages ca

---


## 2.4.0

_2026-07-11_


### New features

* restart dependent services on project-level service restart ([#3103](https://github.com/getarcaneapp/arcane/pull/3103) by `pkoutsovasilis`)
* support for docker override files ([#3104](https://github.com/getarcaneapp/arcane/pull/3104) by `kmendell`)
* increase max session timeout limit to 1 year ([#3222](https://github.com/getarcaneapp/arcane/pull/3222) by `OlziYT`)
* localized timezone based on current locale ([#3238](https://github.com/getarcaneapp/arcane/pull/3238) by `kmendell`)

### Bug fixes

* scope project update backup to changed files and lazy-load file tree ([#3158](https://github.com/getarcaneapp/arcane/pull/3158) by `kmendell`)
* workspace editor showing stale file content after save ([#3188](https://github.com/getarcaneapp/arcane/pull/3188) by `kmendell`)
* don't overwrite good image update records on registry rate-limit errors ([#3190](https://github.com/getarcaneapp/arcane/pull/3190) by `kmendell`)
* skip SMTP auth without full credentials and add none aut

---


## 2.3.2

_2026-07-05_


### Bug fixes

* dialog now owning there own close states, cause mutiple to show at one time([d253fc5](https://github.com/getarcaneapp/arcane/commit/d253fc5d7c081ca43c1385d13da2e127d8bc9e3e) by `kmendell`)
* hide phantom projects from showing in the frontend when deleted ([#3136](https://github.com/getarcaneapp/arcane/pull/3136) by `kmendell`)
* keep modals and menus opaque in dark mode when Glass & Blur is off ([#3139](https://github.com/getarcaneapp/arcane/pull/3139) by `othyn`)
* nested compose files discovery permission issues ([#3096](https://github.com/getarcaneapp/arcane/pull/3096) by `kmendell`)
* ntfy tls regression, image update notifcation flag not be recognized ([#3143](https://github.com/getarcaneapp/arcane/pull/3143) by `kmendell`)
* notification sending rework for reliability ([#3144](https://github.com/getarcaneapp/arcane/pull/3144) by `kmendell`)
* use explicit context for notifications([6987b5e](https://github.com/getarcaneapp/arcane/commit/6987b5e046c3a00c54c88ee54219dc8f

---


## 2.3.1

_2026-07-03_


### Bug fixes

* discard env_file when loading projects to match compose CLI config-hash ([#3100](https://github.com/getarcaneapp/arcane/pull/3100) by `pkoutsovasilis`)
* set explicit gorm LRU cache TTL to avoid constantly rising heap memory ([#3102](https://github.com/getarcaneapp/arcane/pull/3102) by `kmendell`)
* only display memory usage thats non-reclaimable ([#3105](https://github.com/getarcaneapp/arcane/pull/3105) by `kmendell`)

### Dependencies

* bump prettier from 3.9.0 to 3.9.3 ([#3116](https://github.com/getarcaneapp/arcane/pull/3116) by `dependabot`[bot])
* bump `tanstack`/svelte-query from 6.1.35 to 6.1.36 ([#3115](https://github.com/getarcaneapp/arcane/pull/3115) by `dependabot`[bot])
* bump golangci/golangci-lint-action from 9.2.1 to 9.3.0 ([#3106](https://github.com/getarcaneapp/arcane/pull/3106) by `dependabot`[bot])
* bump the tanstack-table group across 1 directory with 2 updates ([#3126](https://github.com/getarcaneapp/arcane/pull/3126) by `dependabot`[bot])
* bump githu

---


## 2.3.0

_2026-07-02_


### New features

* add appearance toggles for blur and interface animations ([#3091](https://github.com/getarcaneapp/arcane/pull/3091) by `othyn`)

### Bug fixes

* edge tunnel go routine leak ([#3073](https://github.com/getarcaneapp/arcane/pull/3073) by `kmendell`)
* users with env only scopes unable to access the UI ([#3081](https://github.com/getarcaneapp/arcane/pull/3081) by `kmendell`)
* use correct disabletlsverification parameter for ntfy ([#3084](https://github.com/getarcaneapp/arcane/pull/3084) by `kmendell`)
* dont create trivy cache in server/client mode ([#3087](https://github.com/getarcaneapp/arcane/pull/3087) by `kmendell`)
* stop phantom and duplicate projects from broken syncs ([#3088](https://github.com/getarcaneapp/arcane/pull/3088) by `kmendell`)
* image polling notifications context being canceled early or not registered at all ([#3089](https://github.com/getarcaneapp/arcane/pull/3089) by `kmendell`)

### Dependencies

* bump svelte from 5.56.3 to 5.56.4 ([#3067](https:/

---


## 2.2.0

_2026-06-29_


### New features

* system mode for light/dark mode ([#2994](https://github.com/getarcaneapp/arcane/pull/2994) by `kmendell`)
* allow use of remote trivy server, and only show fixable cves ([#2999](https://github.com/getarcaneapp/arcane/pull/2999) by `kmendell`)
* preserve managed volumes on project rename ([#2919](https://github.com/getarcaneapp/arcane/pull/2919) by `NeurekaSoftware`)
* show attestations panel for images when supported ([#3036](https://github.com/getarcaneapp/arcane/pull/3036) by `kmendell`)
* add missing kill/pause container actions ([#3037](https://github.com/getarcaneapp/arcane/pull/3037) by `kmendell`)
* add image history, tagging, registry search and local comitting ([#3039](https://github.com/getarcaneapp/arcane/pull/3039) by `kmendell`)
* allow custom profile pictures ([#3023](https://github.com/getarcaneapp/arcane/pull/3023) by `OlziYT`)
* add pre-deploy hook for GitOps project syncs ([#3022](https://github.com/getarcaneapp/arcane/pull/3022) by `peitschie`)

### Bug 

---


## 2.1.0

_2026-06-19_


### New features

* add project file tree management ([#2893](https://github.com/getarcaneapp/arcane/pull/2893) by `NeurekaSoftware`)
* upgrade all environments button ([#2941](https://github.com/getarcaneapp/arcane/pull/2941) by `kmendell`)
* add support for riscv64 ([#2949](https://github.com/getarcaneapp/arcane/pull/2949) by `kmendell`)

### CLI - New features

* add registries create command ([#2874](https://github.com/getarcaneapp/arcane/pull/2874) by `manawenuz`)

### Bug fixes

* fix tables rows not flex redering to use the full table width ([#2928](https://github.com/getarcaneapp/arcane/pull/2928) by `kmendell`)
* add missing healthcheck cli command ([#2929](https://github.com/getarcaneapp/arcane/pull/2929) by `kmendell`)
* allow setting the data directroy for non docker installs ([#2931](https://github.com/getarcaneapp/arcane/pull/2931) by `kmendell`)
* fix dind path mappings for projects and swarm ([#2939](https://github.com/getarcaneapp/arcane/pull/2939) by `kmendell`)
* projects d

---


## 2.0.3

_2026-06-12_


### Bug fixes

* self updater not restarting container properly ([#2897](https://github.com/getarcaneapp/arcane/pull/2897) by `kmendell`)
* dashboard env counts not displaying in a timley manner ([#2901](https://github.com/getarcaneapp/arcane/pull/2901) by `kmendell`)
* user based api keys are capped at users permissions check ([#2918](https://github.com/getarcaneapp/arcane/pull/2918) by `kmendell`)
* serve webhooks from the manager and close edge command allowlist gaps ([#2922](https://github.com/getarcaneapp/arcane/pull/2922) by `kmendell`)
* compose updater not correctly falling back to standalone container update ([#2923](https://github.com/getarcaneapp/arcane/pull/2923) by `kmendell`)
* dont check image updates on locally built images ([#2924](https://github.com/getarcaneapp/arcane/pull/2924) by `kmendell`)

### Dependencies

* bump github.com/nicholas-fedor/shoutrrr from 0.15.1 to 0.16.0 in /backend ([#2867](https://github.com/getarcaneapp/arcane/pull/2867) by `dependabot`[bot])
* bump

---



## 2.0.2

_2026-06-10_

> _Maintenance (2026-06-10):_ hassio-addons base 20.2.0 compatibility — migrated the Traefik helper scripts from the deprecated bashio::addon.* functions to bashio::app.*.

### App build fix

* Fix the image build failing at the Alpine package step on base `20.2.0` (both aarch64 and amd64). The base pins `libcrypto3`/`libssl3` in apk `world` at an exact older revision; once the repo moved to `openssl 3.5.7-r0` (which requires `libcrypto3`/`libssl3=3.5.7-r0`), `apk add openssl` could not resolve against the held libs. Now `apk add --no-cache --upgrade openssl libcrypto3 libssl3 ...` rewrites those world entries and upgrades the whole TLS stack to the repo version in one transaction — no version pinning. (Under apk-tools 3, `apk upgrade --available` does not override an exact world pin, so the libs are listed explicitly.)

### Bug fixes

* update dockerfiles to use correct linker path for version details([725c003](https://github.com/getarcaneapp/arcane/commit/725c0034680aa366dbc8a5e02e827d1057f34ffb) by `kmendell`)
* newly synced git content does not show without a refresh ([#2870](https://github.com/getarcaneapp/arcane/pull/2870) by `kmendell`)
* incorrect height on dashboard cards on smaller screens ([#2878](https://github.com/getarcaneapp/arcane/pull/2878) by `kmendell`)
* add missing swarm-identity endpoint in edge tunnel ([#2886](https://github.com/getarcaneapp/arcane/pull/2886) by `kmendell`)
* activities stream using main context causing app to hang at certain places ([#2887](https://github.com/getarcaneapp/arcane/pull/2887) by `kmendell`)
* x-arcane.icon-light/icon-dark overwriting service-level icons ([#2888](https://github.com/getarcaneapp/arcane/pull/2888) by `kmendell`)
* bind mounts fail to update after git syncs ([#2891](https://github.com/getarcaneapp/arcane/pull/2891) by `kmendell`)
* normalize

---


## 2.0.1

_2026-06-08_


### Bug fixes

* update gomodule imports to /v2([6cb4913](https://github.com/getarcaneapp/arcane/commit/6cb491328f928532213d8efb846e96714dfd4f23) by `kmendell`)
* decrypting of notification tokens failing ([#2850](https://github.com/getarcaneapp/arcane/pull/2850) by `kmendell`)

### Dependencies

* bump updater module to new module name([50c0847](https://github.com/getarcaneapp/arcane/commit/50c084717deeb78179857e52768994d87a3690ac) by `kmendell`)



**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v2.0.0...v2.0.1

---


## 1.20.0

_2026-06-05_


### New features

* add removeOrphans option to project deploy/redeploy ([#2785](https://github.com/getarcaneapp/arcane/pull/2785) by `khanhx`)
* prune idle volume browser helper containers ([#2767](https://github.com/getarcaneapp/arcane/pull/2767) by `Zgrill2`)

### Bug fixes

* slog-go nil pointer dereference ([#2781](https://github.com/getarcaneapp/arcane/pull/2781) by `lohrbini`)
* dashboard card buttons paddings overlaps([c1a0bda](https://github.com/getarcaneapp/arcane/commit/c1a0bda6735a6c50ae989f7e4643ffb09b2edb75) by `kmendell`)
* disable schema display on text selection([058f062](https://github.com/getarcaneapp/arcane/commit/058f062c17329eb43f1968717eff73e715459b79) by `kmendell`)
* clear / check for default jwt secret([ae914bd](https://github.com/getarcaneapp/arcane/commit/ae914bdced852b4c5446a15c1dfbcbd5d6dd50e8) by `kmendell`)

### Dependencies

* bump date-fns from 4.2.1 to 4.3.0 ([#2745](https://github.com/getarcaneapp/arcane/pull/2745) by `dependabot`[bot])
* bump `sveltejs`/ki

---


## 1.19.5

_2026-05-26_


### Bug fixes

* improve environment proxy error handling ([#2649](https://github.com/getarcaneapp/arcane/pull/2649) by `kmendell`)
* align local BuildKit load/push exporter ([#2650](https://github.com/getarcaneapp/arcane/pull/2650) by `kmendell`)
* PUID and PGID being set on project subfolder on every startup ([#2656](https://github.com/getarcaneapp/arcane/pull/2656) by `kmendell`)
* system upgrade doesnt support non unix socket docker hosts ([#2651](https://github.com/getarcaneapp/arcane/pull/2651) by `kmendell`)
* resizing window discards edits in compose editors ([#2719](https://github.com/getarcaneapp/arcane/pull/2719) by `kmendell`)
* only validate project name if it has changed ([#2720](https://github.com/getarcaneapp/arcane/pull/2720) by `kmendell`)
* make Arcane reverse-proxy aware to resolve connection issues ([#2717](https://github.com/getarcaneapp/arcane/pull/2717) by `kmendell`)
* tolerate undefined typed env vars in GitOps sync ([#2721](https://github.com/getarcaneapp/arcane/pu

---


## 1.19.4

_2026-05-19_
### Bug fixes

* block unsafe compose include file reads ([#2630](https://github.com/getarcaneapp/arcane/pull/2630) by `kmendell`)
* add missing gRPC/ws tunnel commands ([#2636](https://github.com/getarcaneapp/arcane/pull/2636) by `kmendell`)
* unable to use templates due to 'not found' error ([#2634](https://github.com/getarcaneapp/arcane/pull/2634) by `kmendell`)
* retry rate limited update checks ([#2639](https://github.com/getarcaneapp/arcane/pull/2639) by `kmendell`)
* prevent projects from disappearing when projects folder is unreadable ([#2641](https://github.com/getarcaneapp/arcane/pull/2641) by `kmendell`)
* release notes not populated for manager instance ([#2643](https://github.com/getarcaneapp/arcane/pull/2643) by `kmendell`)

### Other

* publish manager and agent image tags ([#2645](https://github.com/getarcaneapp/arcane/pull/2645) by `kmendell`)
* use trivy-db mirrors from arcane-tools ([#2646](https://github.com/getarcaneapp/arcane/pull/2646) by `kmendell`)



**Full Changelog

---


## 1.16.4

_2026-03-24_
### Bug fixes

* pin and enforce trivy scanner digest([7975270](https://github.com/getarcaneapp/arcane/commit/7975270059a36e40eb6a2a7fc1d7203f90198bf4) by `kmendell` )



**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v1.16.3...v1.16.4

---


## 1.16.3

_2026-03-16_
### Bug fixes

* docker container creation on api 1.44 attach primary network then remaining networks ([#2053](https://github.com/getarcaneapp/arcane/pull/2053) by `kmendell`)
* add configurable security options for trivy scans ([#2072](https://github.com/getarcaneapp/arcane/pull/2072) by `kmendell`)
* allow configuring whether to prune trivy cache or not ([#2075](https://github.com/getarcaneapp/arcane/pull/2075) by `kmendell`)
* use configured DOCKER_HOST for trivy containers ([#2076](https://github.com/getarcaneapp/arcane/pull/2076) by `kmendell`)
* add missing arcane labels for auto updater ([#2079](https://github.com/getarcaneapp/arcane/pull/2079) by `kmendell`)
* unable to edit env when synced from git ([#2069](https://github.com/getarcaneapp/arcane/pull/2069) by `kmendell`)
* image update inspection fallback to manual vs using mobys distribution inspect ([#2080](https://github.com/getarcaneapp/arcane/pull/2080) by `kmendell`)

### Dependencies

* bump charm.land/lipgloss/v2 from 2.0.0 

---


## 1.16.2

_2026-03-14_
### Bug fixes

* forward and validate origin header in websocket tunnel ([#2003](https://github.com/getarcaneapp/arcane/pull/2003) by @kmendell)
* containers on user created networks not restarted when updated ([#2006](https://github.com/getarcaneapp/arcane/pull/2006) by @kmendell)
* avoid restoring offline environment on app init ([#2011](https://github.com/getarcaneapp/arcane/pull/2011) by @timwedde)
* incorrect volume mount in agent snippets ([#2027](https://github.com/getarcaneapp/arcane/pull/2027) by @kmendell)
* strip `TE: trailers` header to prevent false grpc requests ([#2026](https://github.com/getarcaneapp/arcane/pull/2026) by @kmendell)
* allow yaml merge syntax ([#2033](https://github.com/getarcaneapp/arcane/pull/2033) by @kmendell)
* dialogs in light mode showing too dark([8a29abc](https://github.com/getarcaneapp/arcane/commit/8a29abc4364565e286b43e98c8e49bd079f8315e) by @kmendell)
* build workspace panels using incorrect colors([e46f445](https://github.com/getarcaneapp/a

---


## 1.16.1

_2026-03-12_
### Bug fixes

* explicitly set docker api version based on daemon api version ([#1964](https://github.com/getarcaneapp/arcane/pull/1964) by @kmendell)
* dockerfile_inline builds not working from projects ([#1965](https://github.com/getarcaneapp/arcane/pull/1965) by @kmendell)
* allow rolling back migrations via ALLOW_DOWNGRADE env ([#1966](https://github.com/getarcaneapp/arcane/pull/1966) by @kmendell)
* allow remote git build contexts ([#1968](https://github.com/getarcaneapp/arcane/pull/1968) by @kmendell)
* env variables not resolving in volumes and labels  ([#1970](https://github.com/getarcaneapp/arcane/pull/1970) by @nargotik)
* last used date not being updated for environment api keys([b1f3287](https://github.com/getarcaneapp/arcane/commit/b1f3287efb985f08f4e8dc3e131591486db713b3) by @kmendell)

### Dependencies

* bump github.com/go-git/go-git/v5 from 5.16.5 to 5.17.0 in /backend ([#1917](https://github.com/getarcaneapp/arcane/pull/1917) by @dependabot[bot])
* update frontend d

---


## 1.16.0

_2026-03-07_
### New features

* add grpc support to edge agent tunnel ([#1730](https://github.com/getarcaneapp/arcane/pull/1730) by @kmendell)
* add auto-heal job to restart unhealthy containers ([#1780](https://github.com/getarcaneapp/arcane/pull/1780) by @garrett-edwards)
* editor enhancements, switch back to code mirror editor ([#1861](https://github.com/getarcaneapp/arcane/pull/1861) by @kmendell)
* updated dashboard layout with action items ([#1761](https://github.com/getarcaneapp/arcane/pull/1761) by @kmendell)
* support direct https setup via environment variables ([#1877](https://github.com/getarcaneapp/arcane/pull/1877) by @kmendell)
* selectable trivy container network ([#1896](https://github.com/getarcaneapp/arcane/pull/1896) by @kmendell)
* image build support ([#1687](https://github.com/getarcaneapp/arcane/pull/1687) by @kmendell)
* show template icons based on x-arcane labels ([#1933](https://github.com/getarcaneapp/arcane/pull/1933) by @kmendell)
* oled dark theme ([#1937](https://

---


## 1.15.3

_2026-02-24_
### Bug fixes

* use cpuset instead of cpusnano on synology devices ([#1782](https://github.com/getarcaneapp/arcane/pull/1782) by @kmendell)
* clear image update records by image ID not just repo/tag ([#1809](https://github.com/getarcaneapp/arcane/pull/1809) by @kmendell)
* clear update records by image ID and fail closed on used-image discovery errors ([#1810](https://github.com/getarcaneapp/arcane/pull/1810) by @kmendell)
* bound environment health sync concurrency and prevent overlapping runs ([#1813](https://github.com/getarcaneapp/arcane/pull/1813) by @kmendell)
* track active updates in status maps and bound error-event logging path ([#1817](https://github.com/getarcaneapp/arcane/pull/1817) by @kmendell)
* dont force pull images on project start and respect pull policy ([#1820](https://github.com/getarcaneapp/arcane/pull/1820) by @kmendell)
* registry syncing to environments not running on initially pairing ([#1822](https://github.com/getarcaneapp/arcane/pull/1822) by @kmendell)

---


## 1.15.2

_2026-02-19_
### Bug fixes

* git test connection not using default branch ([#1766](https://github.com/getarcaneapp/arcane/pull/1766) by @kmendell)
* missing settings making env settings not able to be saved ([#1775](https://github.com/getarcaneapp/arcane/pull/1775) by @kmendell)
* change notification logs to TEXT instead of VARCHAR(255) ([#1779](https://github.com/getarcaneapp/arcane/pull/1779) by @kmendell)
* allow trivy container limits to be configured ([#1778](https://github.com/getarcaneapp/arcane/pull/1778) by @kmendell)
* convert cron expressions from utc into TZ var timezone ([#1781](https://github.com/getarcaneapp/arcane/pull/1781) by @kmendell)
* image size mismatch on details page ([#1790](https://github.com/getarcaneapp/arcane/pull/1790) by @kmendell)
* use non-http context for jobs ([#1770](https://github.com/getarcaneapp/arcane/pull/1770) by @kmendell)
* silently refresh token on version mismatch instead of forcing logout ([#1791](https://github.com/getarcaneapp/arcane/pull/1791) by

---


## 1.15.0

_2026-02-14_
### New features

* sync .env files from git repositories ([#1632](https://github.com/getarcaneapp/arcane/pull/1632) by @Icehunter)
* updated table UX, additional 'all' rows option ([#1547](https://github.com/getarcaneapp/arcane/pull/1547) by @cabaucom376)
* container image vulnerability scanning ([#1657](https://github.com/getarcaneapp/arcane/pull/1657) by @kmendell)
* implement container exclusion and prune notifications ([#1635](https://github.com/getarcaneapp/arcane/pull/1635) by @spupuz)
* allow configurable LISTEN address ([#1685](https://github.com/getarcaneapp/arcane/pull/1685) by @kmendell)
* add support for Matrix notifications ([#1679](https://github.com/getarcaneapp/arcane/pull/1679) by @singularity0821)
* inline container exclusion list ([#1693](https://github.com/getarcaneapp/arcane/pull/1693) by @spupuz)
* show projects and containers used by images column ([#1715](https://github.com/getarcaneapp/arcane/pull/1715) by @kmendell)
* move port mappings to networks tab for container details ([#1723](https://github.com/getarcaneapp/arcane/pull/1723) by @kmendell)

### Bug fixes

* ssh git repos commit hash links incorrect ([#1643](https://github.com/getarcaneapp/arcane/pull/1643) by @kmendell)
* x-arcane metadata not allowing variable interpolation ([#1654](https://github.com/getarcaneapp/arcane/pull/1654) by @kmendell)
* inject agent token headers in edge tunnel proxy path ([#1680](https://github.com/getarcaneapp/arcane/pull/1680) by @dathtd119)
* abnormal cpu load climbing over time ([#1652](https://github.com/getarcaneapp/arcane/pull/1652) by @kmendell)
* adjust database connection pool settings ([#1690](https://github.com/getarcaneapp/arcane/pull/1690) by @user00265)
* scan all vulnerabilities causing lag/freezing ([#1694](https://github.com/getarcaneapp/arcane/pull/1694) by @kmendell)
* only send prune summary when resources are pruned ([#1703](https://github.com/getarcaneapp/arcane/pull/1703) by @kmendell)
* OIDC_ENABLED=false not disabling frontend switch ([#1719](https://github.com/getarcaneapp/arcane/pull/1719) by @kmendell)
* table sorting not persisting across reloads ([#1721](https://github.com/getarcaneapp/arcane/pull/1721) by @kmendell)

### Addon fixes

* fix Alpine package version conflict (libcrypto3/libssl3 vs openssl) by upgrading base packages before install
* add default value for BUILD_FROM ARG to silence Docker warning

**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v1.14.1...v1.15.0

---

## 1.14.1

_2026-02-11_
### Bug fixes

* incorrect backgrounds on lightmode ui elements([635e5d0](https://github.com/getarcaneapp/arcane/commit/635e5d0e5f98b0b7001ee2bac51dac155ac3a9dd) by @kmendell)
* align view options dropdown to right side([adac953](https://github.com/getarcaneapp/arcane/commit/adac953ec3853482f8e6ec0ad128792ff6a9e68f) by @kmendell)
* duplicated project/container logs when refreshing log viewer ([#1620](https://github.com/getarcaneapp/arcane/pull/1620) by @kmendell)
* unable to save oidc auto redirect setting([889fb65](https://github.com/getarcaneapp/arcane/commit/889fb65b79a61c3b101e5ea02bd7c089b16b4b00) by @kmendell)
* allow enabling and disabling keyboard shortcuts ([#1623](https://github.com/getarcaneapp/arcane/pull/1623) by @kmendell)
* keyboard shortcuts dont work for non qwerty layouts ([#1624](https://github.com/getarcaneapp/arcane/pull/1624) by @kmendell)
* sync timeout settings to all environments ([#1628](https://github.com/getarcaneapp/arcane/pull/1628) by @kmendell)

### Dep

---


## 1.13.2

_2026-01-20_
> [!IMPORTANT]
> Huge shoutout to @PvtSec for reporting GHSA-2jv8-39rp-cqqr, We recomend upgrading arcane to this version ASAP to fix that issue. 

### Backend - Bug fixes

* apply auth check before proxying request to environments ([#1532](https://github.com/getarcaneapp/arcane/pull/1532) by @kmendell)
* allow HTTP_PROXY and HTTPS_PROXY environment variables ([#1534](https://github.com/getarcaneapp/arcane/pull/1534) by @kmendell)
* use image pull timeout for project pull ([#1533](https://github.com/getarcaneapp/arcane/pull/1533) by @kmendell)
* update color of port badge to be more distinguishable([b0e8b54](https://github.com/getarcaneapp/arcane/commit/b0e8b54ec7c416ef089476106f23c365a74724cd) by @kmendell)

### Dependencies

* bump go version to 1.25.6([501baaf](https://github.com/getarcaneapp/arcane/commit/501baaf7708e8fc83b030650abd04919880da2e4) by @kmendell)
* bump pnpm to 10.28.1([c5ef93e](https://github.com/getarcaneapp/arcane/commit/c5ef93e54db76e44b932953af9f8303

---


## 1.13.1

_2026-01-19_
### Backend - Bug fixes

* ability to resize editor panels horizontally ([#1500](https://github.com/getarcaneapp/arcane/pull/1500) by @kmendell)
* allow oidc endpoints to be defined manually ([#1510](https://github.com/getarcaneapp/arcane/pull/1510) by @kmendell)
* remove file line from db debug logs([fbe204c](https://github.com/getarcaneapp/arcane/commit/fbe204c5ce919282a65313cfc0c889b763eebd64) by @kmendell)
* self update binary path for remote envrionments([974c675](https://github.com/getarcaneapp/arcane/commit/974c675550a0d5408f662d13fe3f8b07edb2267e) by @kmendell)
* generic webhooks do not allow ports ([#1517](https://github.com/getarcaneapp/arcane/pull/1517) by @kmendell)
* logo color not applying on refreshes([fe53985](https://github.com/getarcaneapp/arcane/commit/fe539851d621a35c1ebaa08217151e65bbaae64c) by @kmendell)

### Dependencies

* bump @sveltejs/kit from 2.49.4 to 2.49.5 in the npm_and_yarn group across 1 directory ([#1492](https://github.com/getarcaneapp/arcane/pull/1

---


## 1.12.2

_2026-01-14_
> [!IMPORTANT]
> Sorry for the double release, this release however should fix the path issues by making all projects directories absolute paths instead of relative paths.

### Backend - Bug fixes

* template editor heights being cutoff([7057deb](https://github.com/getarcaneapp/arcane/commit/7057deb42174cef218c623b1c431546c4a771396) by @kmendell)
* double label text on template buttons([6316833](https://github.com/getarcaneapp/arcane/commit/6316833c79f5b3e17c194c701ddc1446cab0b038) by @kmendell)
* use full absolute path for projects directory ([#1409](https://github.com/getarcaneapp/arcane/pull/1409) by @kmendell)
* editor cursor misalignment ([#1412](https://github.com/getarcaneapp/arcane/pull/1412) by @kmendell)



**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v1.12.1...v1.12.2

---


## 1.11.3

_2026-01-04_
### Initial Release

- Initial Home Assistant addon release
- Based on Arcane v1.11.3
- Features:
  - Container management with real-time stats
  - Docker Compose stack management
  - Resource monitoring with graphs
  - Image, volume, and network management
  - Automatic container image updates
  - Modern, mobile-friendly UI
  - Home Assistant ingress integration
  - Persistent data storage

### Arcane v1.11.3 Release Notes

For full upstream release notes, see: https://github.com/getarcaneapp/arcane/releases/tag/v1.11.3

---
