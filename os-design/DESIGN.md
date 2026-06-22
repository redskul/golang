# Strata: An Architecture Proposal for a Post-RHEL/Linux Distribution

## 0. Scope and intent

This is a design document, not an implementation. It analyzes four
recurring failure modes of mainstream enterprise Linux (using RHEL and
its ecosystem as the reference point, since most of these problems are
shared across Debian/Ubuntu/SUSE too) and proposes a concrete
architecture — "Strata" — that addresses them. Strata is not a new
kernel; it reuses the Linux kernel (rewriting a kernel to fix
distribution-layer problems is solving the wrong layer). What changes
is everything above the kernel: how packages are built and installed,
how updates are shipped and rolled back, how security patches reach
machines, and how the support/governance model is structured.

The four problem areas, in the order requested:

1. Package / dependency management
2. Security & patching cadence
3. Licensing / support model
4. Reliability / upgrade safety

Each section follows the same shape: **current pain → root cause →
proposed mechanism → tradeoffs**.

---

## 1. Package & dependency management

### Current pain
- RPM/yum/dnf (and apt, to a lesser degree) resolve dependencies at
  install time against a mutable system state. Two machines that
  installed the "same" packages over time can end up with different
  files on disk depending on install order, repo snapshots available
  at the time, and manual interventions.
- Dependency resolution is global and singular: you cannot have package
  A linked against OpenSSL 1.1 and package B linked against OpenSSL 3.0
  on the same root without containers or modularity hacks (RHEL's
  "Application Streams" partially address this but add their own
  combinatorial complexity).
- Removing a package can silently break others through shared,
  loosely-versioned dependencies. `rpm -e` and `dnf remove` resolution
  is asymmetric with install resolution.
- Builds are not reproducible: the same SRPM rebuilt six months apart
  can produce a bit-for-bit different binary, making supply-chain
  verification impossible.

### Root cause
RPM models the system as a single mutable bag of files with a
dependency graph layered on top *after the fact*. There's no single
source of truth for "what does this system look like" — the truth is
whatever `rpm -qa` reports right now, which is a function of history,
not a function of declared intent.

### Proposed mechanism
- **Content-addressed package store**, modeled on Nix/Guix: every
  package is built deterministically and stored at a path keyed by the
  hash of its full build closure (sources + build inputs + build
  script), e.g. `/strata/store/<hash>-openssl-3.2.1/`. Multiple versions
  of the same library coexist on disk with zero conflict because they
  never share a path.
- **Declarative system manifests**: a machine's entire userspace is
  described by a single manifest file (think: a lockfile) that lists
  exact store paths for every package. `strata apply manifest.toml`
  computes the diff between current and desired store-path sets and
  performs only the necessary store fetches/builds plus an atomic
  symlink-swap of the active profile — no in-place mutation of
  installed files.
- **Reproducible builds as a hard gate**: the build infra rejects any
  package whose build is non-deterministic (sandboxed builds, no
  network access during build, normalized timestamps). This is what
  makes content-addressing trustworthy in the first place.
- **Dependency resolution becomes a build-time/CI-time problem**, not a
  machine-time problem. Conflicts are caught when the manifest is
  composed (in CI, against a lockfile), not when a sysadmin runs
  `dnf update` on a production box at 2 AM.

### Tradeoffs
- Disk usage is higher (multiple versions coexist) — mitigated by
  store-level deduplication and garbage collection of unreferenced
  paths.
- Steeper learning curve for admins used to imperative `yum install
  foo`. Strata should ship a thin imperative-feeling CLI
  (`strata install foo`) that's secretly generating/applying a manifest
  diff under the hood, so day-to-day ergonomics don't regress.
- Build infrastructure cost is higher than RPM's (rebuilding closures
  instead of patching binaries), offset by aggressive binary caching.

---

## 2. Security & patching cadence

### Current pain
- CVE-to-patch latency for RHEL/CentOS-derivatives is often measured in
  days to weeks for non-kernel packages, and kernel CVEs frequently
  require a reboot, which enterprises batch into monthly/quarterly
  maintenance windows — leaving known-exploitable systems live for a
  long tail.
- SELinux is powerful but operationally avoided: its policy language
  is opaque enough that the most common admin response to a denial is
  `setenforce 0`, which defeats the entire control.
- Patch application and patch *verification* are different problems
  that current tooling conflates: `dnf update` tells you packages were
  installed, not that the vulnerable code path is no longer reachable
  at runtime.

### Root cause
Security tooling in current Linux is bolted onto a generically mutable
system (same root cause as section 1) and treats "patched" as a
package-manager-state question rather than a runtime-state question.
Live patching exists (kpatch, Ubuntu Livepatch) but is an opt-in
side-channel, not the default path.

### Proposed mechanism
- **Live-patch-first kernel update path**: kernel CVE fixes are
  shipped as kpatch-style hot patches by default; a full kernel
  image swap (via the atomic update mechanism in section 4) happens on
  a normal cadence, but the *security-relevant* fix lands without a
  reboot. Reboots become a reliability/performance event, not a
  security event.
- **Runtime attestation, not just install attestation**: a lightweight
  in-kernel/eBPF agent maps loaded code (libraries actually mapped into
  running processes, kernel modules actually loaded) against the CVE
  database and reports *exploitability*, not just *package version*.
  This directly answers "are we still vulnerable" instead of "did the
  installer run."
- **Mandatory Access Control with generated, not hand-written, policy**:
  instead of admins writing/relaxing SELinux policy, policy is
  generated from the declarative manifest (section 1) — since the
  manifest already enumerates exactly which binaries, files, and
  capabilities each package needs, a default-deny policy can be
  derived automatically and only needs human override for genuinely
  novel cases. This removes the "policy is too hard, disable it"
  escape hatch by making the easy path the secure path.
- **SLA-backed patch SLOs published as machine-readable data**: every
  CVE affecting the distro gets a published target remediation time by
  severity (e.g., Critical: 24h via live-patch, High: 7d, Medium: next
  point release), and the patch feed includes a queryable timestamp so
  compliance tooling can verify the SLO was met — not just trust a
  blog post.

### Tradeoffs
- Live-patching kernels is constrained: large structural changes still
  need a real reboot. The model degrades gracefully to "schedule a
  reboot" but should never *silently* fail to apply a critical fix.
- Auto-generated MAC policy needs a robust "this is genuinely new
  behavior, allow once" workflow, or it becomes the new `setenforce 0`
  under operational pressure.
- Runtime attestation agent is more privileged code running in the
  kernel — itself attack surface that must be held to a higher bar
  than the things it's protecting.

---

## 3. Licensing & support model

### Current pain
- The RHEL ecosystem fractured visibly after CentOS's shift to Stream
  and RHEL's subsequent SRPM-access restriction: downstream rebuilders
  (Rocky, Alma) now operate in legal/operational uncertainty, and
  enterprises face a binary choice between paying Red Hat subscription
  fees per-node or accepting rebuild risk.
- Subscription cost scales per-node in a way that's disconnected from
  actual support consumption — a fleet of 10,000 mostly-idle VMs costs
  the same per-unit as 10,000 heavily-supported production nodes.
- "Open source" and "open governance" are conflated in marketing but
  are not the same thing: RHEL's source has historically been
  available, but *influence over the roadmap and access to
  pre-release fixes* has not been open in the same way.

### Root cause
The business model is built around gating *access to artifacts*
(binaries, SRPMs, early CVE fixes) rather than charging for the thing
that actually costs money to provide: human support, SLAs, and
certification. Gating artifacts is what creates downstream rebuilder
conflict, because artifact-gating is trivially circumvented by anyone
willing to rebuild from source — so the gate mostly punishes good-faith
downstream projects rather than bad actors.

### Proposed mechanism
- **Decouple distribution from support entirely.** All artifacts
  (binaries, build closures, signed packages, CVE patches) are public
  and freely redistributable with no embargo period — this is the
  Debian/Fedora model, not the post-2023 RHEL model. There is nothing
  to "rebuild" because there's nothing gated to begin with.
- **Charge for what's actually scarce**: certified support contracts
  (named SLA, security audit trail, indemnification, hardware
  certification matrices, long-term support backporting labor). This
  is a services/labor business, not an artifact-access business, and
  it doesn't create an adversarial relationship with the community that
  builds and tests the OS.
- **Governance lives in a foundation, not a single vendor**, with a
  published RFC process for ABI/API stability commitments (similar in
  spirit to how OpenTofu/Linux kernel governance work). A single
  commercial entity can still be the primary maintainer and sell
  support, but roadmap decisions that affect long-term compatibility
  go through a process the community can see and contest *before*
  it ships, not after.
- **Cost model tied to support tier consumed, not node count**: e.g.
  pooled support-hours or severity-weighted incident billing for
  fleets, with a genuinely free (not crippled, not time-bombed) tier
  for self-supported use, including production use. The free tier
  funds itself through the paid tier's actual differentiator (support
  labor), not through artificial scarcity of bits.

### Tradeoffs
- This is a harder business to run profitably than artifact-gating,
  which is precisely why incumbents drift toward gating over time —
  any implementation has to be honest that this trades short-term
  revenue predictability for long-term ecosystem trust.
- Foundation governance is slower than single-vendor decision-making;
  the RFC process needs a real fast-path for security fixes so
  governance doesn't become a synonym for "CVE response delay."

---

## 4. Reliability & upgrade safety

### Current pain
- In-place major version upgrades (`leapp`, `do-release-upgrade`) are
  the highest-risk operation an admin performs on a long-lived system,
  because they mutate a live root filesystem with no atomic
  all-or-nothing guarantee — a failure partway through can leave a
  system in a state that is neither the old nor the new version.
- There is no cheap, fast rollback for "the update applied cleanly but
  broke something." Recovery means restoring from backup/snapshot
  infrastructure that may not exist, or may be hours old.
- systemd's scope (init, logging, networking, device management, login,
  time sync, DNS resolution stub, etc.) means a regression in one
  subsystem can cascade into seemingly unrelated boot failures, and the
  unit dependency graph for a complex server is hard for any one admin
  to reason about end-to-end.

### Root cause
Updates are implemented as *mutations of a live, in-use root
filesystem*. Any operation that mutates state you're simultaneously
depending on cannot be cleanly atomic or cleanly reversible — this is
the same root cause as sections 1 and 2, applied to the whole-system
upgrade case rather than the package case.

### Proposed mechanism
- **Image-based, A/B atomic updates** (the model proven by Fedora
  CoreOS / Flatcar / ChromeOS / Android): the OS root is a read-only,
  versioned, signed image. Updates download/build the next image
  into the *inactive* slot, verify it fully, and only flip the boot
  target on success. A failed update never touches the running system.
  Rollback is "boot the other slot," not "restore from backup" —
  available instantly, not after an RTO.
- **State is explicitly separated from the OS image**: `/etc`
  overrides, application data, and local config live in a layer that's
  reconciled (not overwritten) across image boundaries, with the
  reconciliation rules declared per-file (keep-local, take-new,
  three-way-merge) rather than improvised per-upgrade the way
  `.rpmnew`/`.rpmsave` files are today.
- **Health-checked boot with automatic rollback**: after flipping to a
  new image slot, a bounded health-check window (service start
  success, network reachability, a configurable smoke-test hook) must
  pass before the new slot is marked "good." If it doesn't pass within
  the window, the bootloader automatically reverts to the previous
  slot on next boot — no human has to be paged at 3 AM to manually
  decide to roll back a bad kernel update.
- **systemd's blast radius is reduced by contract, not by replacement**:
  rewriting init is out of scope and not worth the disruption, but
  Strata pins and tests systemd as part of the atomic image (so a
  systemd regression is caught by the same staged-rollout health checks
  as everything else) and documents/enforces narrower unit dependency
  graphs for first-party services so failure domains stay legible.

### Tradeoffs
- Image-based updates trade flexibility (you can't just `vim` a file
  in `/usr` and have it stick) for safety — this is a deliberate and
  correct tradeoff for servers, but needs a clearly documented
  "developer mode" escape hatch (an explicitly-named, separately
  tracked overlay) so it doesn't just push ad-hoc mutation into a less
  visible place.
- Bigger update payloads (full image deltas vs. individual RPM deltas)
  need binary-diff transport to stay bandwidth-reasonable at fleet
  scale.
- Health-check-gated auto-rollback needs to be conservative about false
  positives (rolling back a *good* update because a flaky smoke test
  failed is its own reliability problem).

---

## 5. How the four pieces compose

These aren't four independent features — they share one underlying
idea: **make system state a deterministic function of a declared
intent, never a function of history.**

- Section 1's content-addressed store is what makes section 2's
  auto-generated security policy possible (the manifest already knows
  what each package needs).
- Section 1's manifest is what makes section 4's image builds
  reproducible and diffable.
- Section 4's atomic slots are what make section 2's live-patch model
  safe to fall back from (a live-patch that goes wrong is bounded by
  the same A/B rollback as a full image).
- Section 3's open-artifact model is what makes sections 1, 2, and 4
  independently verifiable by anyone, instead of requiring trust in a
  single vendor's say-so.

## 6. Phased roadmap (illustrative, not a commitment)

| Phase | Deliverable | Depends on |
|---|---|---|
| 0 | Reproducible build infra + content-addressed store for a small base package set | — |
| 1 | Declarative manifest CLI (`strata apply`) targeting a non-production VM image | Phase 0 |
| 2 | A/B atomic image format + health-checked boot/rollback | Phase 1 |
| 3 | Auto-generated MAC policy from manifests; eBPF runtime CVE-exposure attestation | Phase 1 |
| 4 | Live-patch pipeline wired to the published CVE SLA feed | Phase 2, 3 |
| 5 | Open governance foundation + support-tier business model launch | Phases 0–4 stable |

## 7. What this proposal deliberately does not do

- It does not propose a new kernel, init system, or C library — the
  problems analyzed here live above that layer, and replacing
  well-tested low-level components would trade known problems for
  unknown ones without addressing the actual complaints.
- It does not claim zero migration cost from RPM-based systems; a
  real implementation would need a compatibility/import path (e.g.
  ingesting an existing RPM database into an initial manifest) that is
  out of scope for this document.
