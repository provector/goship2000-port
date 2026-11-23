# Port save format — C.1 plus Step D preferences

The optional config `saves` path defaults to APP/saves. It must be separate from
original inputs and generated assets. Close the program before editing JSON.
Normal startup prints the actual save path and selected pilot/rank; development
commands use newly created scratch saves instead.

## Ownership

`roster.json` schema2 owns exactly six ordered stable-ID references (empty string
means empty slot), `selected_pilot_id`, a technical `revision`, and immutable
legacy-import references. It contains no rank, OfficeState or pilot progression.

`pilots/<id>.json` schema1 owns that pilot's flat typed fields: `id`, `name`,
`squadron`, `emblem` (0..17), `rank`, `status`, `mission_type`, `theatre`, supported
opaque eligibility state, `statistics`, and an optional retained original record.
The retained record is a witness/preservation channel, not an override for edited
typed fields. Other original state remains opaque rather than guessed.

Enums use stable names. Ranks in order are warrant_officer_candidate,
warrant_officer_1, chief_warrant_officer_2, chief_warrant_officer_3,
chief_warrant_officer_4, second_lieutenant, first_lieutenant, captain, major,
lieutenant_colonel, colonel, brigadier_general. Status is active, inactive or
legacy_unknown. Mission names are training, single, simulated, flight, campaign;
theatres are central_europe, persian_gulf, antarctica, philippines. Eligibility
rules still apply; listing an enum does not unlock a branch.

Statistics are nullable: null means unknown, not zero. New pilots have known zero
statistics. Fields are signed32 `last_mission_score`, `best_mission_score`,
`career_score`, `mission_campaign_counter`, and `phoenix_counter`. The packed
mission/campaign field displays signed remainder100 as missions and quotient100
as campaigns. Phoenix displays the negated signed low16 word of its accumulator.
Optional `award_counters` has eight signed bytes; omission means unknown. They
have source-backed icons/spacing, without guessed award names.

Missing pilot files, duplicate IDs, unsafe IDs/paths, invalid enums, future
schemas and malformed input are errors. Do not repair an error by deleting saves.
There is no live filesystem watcher; restart after an external edit.

## Import and migration

First initialization read-only imports a supported2030-byte legacy roster using
the recognized executable profile. Exact bytes/provenance are retained under
imports/<sha256>/, including the separate seventh290-byte block. The first Active
occupied slot is selected by documented port policy. No roster means truly empty
state. Existing port saves always win over old imported data.

An old Step C office-save.json is archived with the **entire** old directory and
hash manifest in step-c-before-migration/ before publication. Typed edits, order,
selection and opaque raw bytes are retained. Newly understood statistics are
derived from original bytes only when the old typed JSON never represented them.
Explicit newer values or unknown/null statistics remain authoritative. Old
chronological backups remain inactive and are not mission snapshots. Completed
archives are verified on retry. An existing migrated roster prevents migration;
missing roster with surviving pilot files refuses stale reinitialization.

## Mission recovery

Only `pilots/<id>.backup.json` is a gameplay backup, for that pilot alone. The
explicit development/legacy operation PrepareMissionSnapshot requires a selected
Active pilot and known statistics and stores its current complete pilot state.
Browsing/preflight/launch does not create this file. N.1 final acknowledgement
publishes completed-mission recovery as specified below. There is no player debug-
backup button.

Recovery previews only the selected pilot's backup. Cancel changes nothing.
Confirm restores the backed-up pilot state, subtracts one from its saved signed32
Phoenix accumulator (original modulo32 arithmetic), consumes the backup and
refreshes the office. It does not make a backup of the replaced state. Other
pilots, roster order and selection remain intact. Repeat recovery is unavailable,
including after restart, until a later completed mission publishes another snapshot. The
legacy seventh block is preserved but not automatically assigned to a pilot.

The manufactured recovery fixture is external. Its historical location in the research checkout was
`working/stepC1/save-fixtures01/recovery-fixture`. Copy it into a
new mutable saves directory before testing. It is not normal user progress or an
observed original mission result.

## Write guarantees

Same-directory temporary files are synced then renamed. A bounded
.pending-commit.json holds before hashes and after bytes for changed files, with
pilot publication before roster publication and backup consumption in the same
transaction. Restart rolls forward only when each file matches its known prior
or already-published after state; conflicting external edits cause an error.
This minimal technical journal is not another gameplay backup/history system.

A writer lock rejects concurrent updates; only a proven-dead owner is reclaimed
automatically. An unverifiable lock requires inspection. Save/import directories
reject symlinks. Multiple renames are not globally atomic, and ordinary Sync/
Rename without directory fsync does not promise universal power-loss durability.
If a write reports an error after journaling, preserve all files: restart may
complete the already recorded transaction. Never manufacture a replacement save.


## Step D mission-session ownership

Pilot schema1 gains optional `preflight_settings`; older schema1 pilots remain
readable without migration. It owns seven UI choice ordinals, validated against
the recovered controls:

```json
"preflight_settings": {
  "cpg_controls": 0,
  "enemy_quality": 0,
  "flight_model": 0,
  "landings": 0,
  "avoidance": 0,
  "wind": 0,
  "visibility": 0
}
```

CP/G permits0..2, enemy quality0..3, others0..1. These are source UI ordinals,
not promised simulation effects. If absent, import the known legacy bytes
+5f,+5d,+5e,+60,+61,+62,+63 in that order. Missing or invalid source values cause
a useful error; they do not become guessed defaults. Once typed settings exist,
they override the retained legacy witness, as rank and other typed fields do.

Generated data is normally **session-only**. Explicit Campaign Suspend additionally stores the source-linked mission and preparation rows described below; derived prose, commander/paper renderer state and RNG traces are never saved.
The selected pilot is bound to the session. **Step E:** returning from settings
(Escape) validates and commits edited preferences to that pilot immediately.
This is a port persistence adaptation: original C024c copied the seven choices
to pilot memory/selected slot, then outer A1c84d saved the whole records later.
New pilots receive the recovered all-zero choice defaults. Imported missing or
corrupt settings still error rather than being silently reset. Quit does not
commit an unfinished settings draft; quitting after a successful local return
retains its already committed preferences. No preparation action
does not create the completed-mission recovery snapshot. Later flight/result
behavior is governed by the N.1 contract below.

Pilot preferences and a suspended mission's settings are different owners.
Resume restores the prepared snapshot's settings while leaving pilot preferences
intact. Closing an unchanged snapshot does not overwrite newer preferences.
An edited settings return commits its complete seven-value choice set without
rewriting the saved suspension; explicitly suspending again records a new mission
snapshot. No extra renderer state or preference document is introduced.

Single/Flight decline performs the established conditional signed16 opaque +59h
addition (`if value<3000: value+=300`, including2999→3299), and shifts prior
scenario +5b→+5c before storing the new suffix +5b. Known +58 receives SNHD[0].
Training skips that counter/history addition. These small original-record writes
are explicit preserved-state operations, not newly named rank/score fields.
Campaign now uses the optional typed contract below. Editor remains unavailable; no successful campaign history is manufactured.


## Step D.2 additive pilot fields

Roster schema2 / pilot schema1 remain readable: optional fields do not require a
migration or rewrite on load. `squadron_pilots` owns four stored wingmen, each with
`name`, `rank`, `portrait` and optional exact41-byte `legacy_record_base64`. When
absent, the four rows at main legacy+55h+41*i (i=1..4) supply them. The notional row0
is not another pilot. Typed fields are authoritative; raw award/status/opaque
words are preserved, not relabelled as AI skill. Normal rank edits never initialize
or reroll wingmen. Outer preparation result saves copy back the assigned records.

Optional `campaign` version1 owns profile identity, theatre, active/suspended phase,
target/result/loss-related counters, selected/prior scenario and ordinal,
source-consistent scenario formation caches and exact retained provenance/reserved
rows. These typed values supersede corresponding old legacy witnesses. They are
not an invented stock/supply system. Unknown history and metadata spill into
unestablished adjacent state are rejected.

`suspended_preparation` stores the complete supported source-linked mission,
accepted objects/paths/objective references/conditions, restriction lists,
five aircraft/loadouts, the five *initial* aircraft IDs, compact flight assignments
(`source_index`, `group`, `leader` only),
seven settings and transient secondary control. Default aircraft IDs must remain
separate from edited rows. Optional uninterpreted source preparation blocks from
a legacy import are retained verbatim. Current pilot names/ranks/portraits/awards remain
authoritative after resume; display labels and41-byte presentation rows are rebuilt
and never serialized into a suspended flight. Stored wingmen copy back by unique
source identity rather than displayed assignment order.
No pixel buffers, generated prose, filesystem DOS cache API or executable addresses
are required to load a native session.

Fresh preparation is a draft. Suspend commits its complete state; resume restores
it without regenerating objects/equipment/restrictions. Decline increments the
source counters (300 conditional opaque-word addition, byte-wrapped target and
second result counters), then prepares another Campaign. Abort applies the source
500-word operation, clears Campaign and selects Flight. These are not successful
mission outcomes. Equipment/paper returns do not save; settings commits edited
per-pilot preferences as described above. Quit is not automatic suspension.
Saved suspend/decline/abort/preferences use the existing journal/lock transaction.
No pre-flight screen or normal launch creates a `<id>.backup.json`; N.1 publishes
it only with an acknowledged completed result, as specified below.

Step E prepares the next declined-Campaign candidate before persistence or
publication. Generation failure retains the current acknowledged mission and save.
A save error closes preparation and blocks menu mutations until restart; Alt+Q
remains available. A pending journal may finish on restart, so this is not an
all-files atomic rollback or a power-loss durability claim. No unsupported cached
reference is clamped or replaced with invented history.

`import-campaign` requires an explicit matching legacy roster/full snapshot and
optional associated intermediate cache and a non-existing destination. It retains
all source files and hashes separately under imports. It cannot overwrite an
existing authoritative port save. Incomplete states produce diagnostics; no
legacy bytes or user saves are silently repaired.


## Step N.1 session abandonment and completed-mission recovery

N.2 leaves this schema and commit/recovery policy unchanged. Live TARGET62,
pickup assignments, carried SHIP flags, objective limits and FARP servicing are
transient initialized-mission state. They survive in-process GPS/view transitions,
but do not resume an unfinished sortie after quit/restart. Tactical Flight mechanics
do not widen persistent Single/Training result admission or create follower metrics.
No objective/service/render callback commits a result or consumes Phoenix.

Optional schema2 `pilots/<id>.sortie.json` references `sorties/<32-hex-id>.json`.
Launch binds accepted pilot/profile/DF6 clock and mission/launch hashes without
creating, replacing or consuming Phoenix. Pending facts/proposals/choices are
immutable transaction inputs for the current process, not resumable UI/flight.
One-player Single/Training result admission and pure scoring remain unchanged.

**The commit boundary is the final debrief acknowledgement.** It publishes a
checked durable `.pending-commit.json` containing the final pilot, pre-result
accepted pilot as `<id>.backup.json`, consumed sortie identity and roster revision.
An existing backup's hash, including its absence, is guarded. Known conflicts in
any remaining journal payload are checked before another payload is applied;
each write is checked again. Recovery completes the exact selected bytes, without
rescoring. This is a restart-recoverable multi-file journal, not an all-files
filesystem atomic operation or an unconditional power-loss guarantee.

Before that boundary, confirmed Alt+Q, window close/process loss and next startup
never create results. Startup first finishes any already durable save journal,
then moves unfinished/corrupt/missing-reference optional session records into
`abandoned-sessions/`, retaining their bytes and explanatory sidecars. It neither
restores `PilotBefore` over canonical data nor changes legitimate prior settings,
roster edits or decline penalties. Normal startup reaches the office, with normal
startup presentation; no forced interrupted/debrief page is restored. A stale
stage/choice/commit callback cannot revive an archived sortie. A valid consumed
marker remains available for duplicate detection after later sorties and Phoenix.

Schema1 Step N records are explicitly recognized: valid committed identities are
retained; active/pending/unsupported UI records and dangling references are
archived on startup. A historical backup is preserved rather than guessing
whether an older build created it at launch. An already durable legacy journal
finishes its recorded operations, including historical backup deletion if that
was already acknowledged. N.1 never synthesizes a replacement historical backup.
New completed results publish the corrected backup ownership. Corrupt optional
JSON does not make a valid pilot unusable. Corrupt canonical pilot/roster/journal,
unsafe symlinks, permissions and conflicting external edits still fail safely;
no defaults or fabricated awards repair them.

Explicit duty-roster recovery replaces only the selected pilot with its completed
mission's pre-result snapshot and source-decrements the signed32 Phoenix counter.
It consumes that backup once, preserves unrelated pilots/roster/settings ownership
and retains all consumed sortie identities, so an old result cannot be re-awarded.
An unfinished next sortie never removes the previous completed recovery.
`PrepareMissionSnapshot` remains an explicit development/legacy operation; normal
flight entry does not invoke it.

The final-ack boundary and optional archive are **native reliability/user policies**.
Original GS saves base score before debrief and can save later choices separately;
N.1 does not claim identical DOS late-quit timing. Original explicit recovery and
Alt+E versus Alt+Q meanings remain separate. See the
historical N.1 policy and cut-point evidence (research archive, `evidence/stepN1/session-policy.md`).
Canonical roster/pilot schema versions are unchanged. Opening existing valid
canonical saves does not rewrite them merely to apply this optional migration.

## N.3 transient damage boundary

Damage12/encoded health and the exact source terminal cause/status belong only to
the current sortie. Frozen transient endings copy them by value; they do not add
persisted career fields or widen result schemas/admission. Any nonzero player
damage is explicitly unscored pending N.4 normalization. No death/rescue/capture
flag, penalty, Phoenix or award is fabricated. Alt+Q/process loss still abandons
unfinished play under N.1; final-ack journals/consume-once IDs/conflict checks stay.
Development `dev -scenario` requires a fresh separate app-dir and writes a scratch
pilot under dev-output; it never restores an unfinished personal sortie.

## Step N.4 result extension (supersedes earlier admission limits)

Sortie record/reference schema3 reads schemas1/2. Pending records add optional
`crew_choices` (source actor recipients); the immutable proposal adds version1
`result_context` and ordered result details. Legacy healthy one-player proposals
remain decodable. Active/pending optional sessions still abandon/quarantine on
startup; schema3 does **not** restore unfinished play. Durable journals still
finish exactly once before ordinary loading.

Frozen context owns actual accepted actor/SourceIndex/callsign/group/rank and
SHIP30, A/B performance, health masks, OBJECT80 dynamic kinds/poses, TARGET62,
launch pose/scenario suffix/ordinal and the actual accepted Campaign draft.
Raw0268 is rebuilt from source final kinds, not live AE9. GS result randomness
uses a separate 32-bit CRT LCG. Native policy seeds it once from SHA256 of the
stable sortie identity; retained seed/state/draw sites make the immutable
proposal reproducible. Supplied original seeds are supported by isolated tests.
Rendering/reopening a pending page does not reroll it.

Supported fresh Single/Training, known Flight crew and bounded officer Campaign
results may now include damaged returns and source-normalized MIA/KIA/rescue.
Pilot schema1 adds `mia`/`kia` status strings with exact raw56=3/4 ownership.
Known old `legacy_unknown`2/3/4 plus matching290-byte witness upgrades in memory;
loading does not rewrite those files. Unknown statuses remain unknown. Unavailable
pilots cannot launch; Phoenix uses the actual pre-result backup and retains all
consumed result identities after recovery.

Final acknowledgement atomically publishes the resolved player, real crew
choices/replacements, Campaign clone/conclusion, pre-result Phoenix and consumed
sortie marker. Loss presentation itself never commits. No whole-roster rollback,
re-award after Phoenix, fabricated follower metrics or default loss classification.
External-change and interruption checks remain in the existing journal. The
separate roster-wide MIA-objective rescue branch and unresolved imported/reserved
rescue/Campaign crossmodes stay excluded; see evidence/stepN4/remaining-limits.md.

## N.5 — versioned scoring and bounded multi-pilot recovery

The current optional sortie record/reference schema is **4**, reading1/2/3/4.
Pilot/roster schemas and canonical per-pilot ownership are unchanged. New starts
pin `proposal_version:2`; missing version in historical starts means1. A schema4
start must explicitly pin2. Version1 proposals are strictly rederived with the
historical calculator, including N.4's positive-follower doubling. Version2 adds
each follower once; player positive doubling and other source arithmetic remain.
New launches cannot stage version1 proposals. Already committed careers are not
recalculated, and old valid consumed records/Phoenix remain readable.

Durable old journals finish their exact desired bytes once; validation does not
reroll or replace their proposal. Uncommitted old/new metadata is abandoned and
quarantined on startup under N.1 policy. Corrupt optional metadata never fabricates
progression. Canonical conflicts and durable journal conflicts remain explicit.

New launch metadata can retain `recovery_roster`: full, fingerprinted MIA records
in a virtual source POST order (posted pilot moved to row0). Native stable roster
slots are not reordered. The frozen result context includes `mission_recovery`
with full immutable launch TARGET62 separately from live final TARGET62, real
objective-assignment owners and that same candidate snapshot. Dropping the owner
context from a new launch with known MIA candidates is invalid.

Policy `native-launch-target-unique-mia-v1` is an explicit compatibility repair
for the original GS TARGET loader dependency. Only a unique known MIA matching
the launch theatre/scenario, a supported completed category2 objective, assigned
player cargo18 and the player's actual safe-return80 are admitted. Changed TARGET
identity/tails, ambiguous candidates and unknown required provenance are excluded.
Other result paths without a matching MIA are not rejected by that restriction.
No crash position or particular missing-pilot mission placement is invented.

The proposal's `recovered_pilot` retains a before fingerprint and complete
before/after records. Its status-only
change and `committed_recovered_pilot_sha256` share the player's final-ack journal.
Both canonical records must still match their snapshots. The peer receives no
player score/medal/counter update, and its own Phoenix and consumed sortie reference
are retained. Source special congratulations bypass the later ordinary JUMP rank
action; earlier commander acceptance remains ordered separately.

Roster viewing is transient and independent of posted launch identity. Targeted
Phoenix uses the inspected stable ID and backup hash; it does not post or revive
an unavailable pilot merely to inspect it. Explicit recovery changes only its
owner and consumes that backup once. It never clears consumed result identities.

After final acknowledgement, an admitted ongoing Campaign stages one genuine next
preparation/briefing. Replay/library is temporarily skipped. Next preparation
failure cannot undo the completed result or publish a partial draft. Closing an
uncommitted next briefing abandons only that new play; restart opens the saved
office, without forced session resumption. The journal remains the logical atomic
commit protocol, not a claim of simultaneous filesystem writes across files.

## N.6 Campaign cache knowledge (nested Campaign version2)

Canonical pilot/roster schemas and sortie proposal versions are unchanged.
Campaign version1 remains readable and its historical result calculators retain
that exact representation. A new preparation **clone** upgrades a version1
Campaign to version2; it does not rewrite old proposals, canonical careers,
consumed IDs, Phoenix or an already durable journal.

Version2 adds optional `cache_inventory` entries with byte-domain `ordinal`,
`presence` (`present`, `absent`, `unknown`) and a nonempty evidence label.
An omitted entry is unknown unless an exact cache payload is present. Fresh
native Campaigns own a new isolated namespace and explicitly know slots0–9 are
absent; successful source preparation records the created ordinal as present.
Old native-owned version1 histories retain their established fresh namespace.
Old imports (`import_sha256` present) do **not** infer that omitted files were
absent: only exact supplied payloads become known-present. An invalid duplicate,
contradictory presence/payload or version1 carrying version2 meaning rejects.

The actual selected-ordinal preparation lookup checks this knowledge. Unknown
rejects the next candidate without publishing a draft/cache or undoing the prior
completed result. Explicit verified absence follows source miss/generation.
`BindCampaignIntermediate` binds complete raw bytes to an exact prepared current
read context, preserving unknown creation provenance; it never replaces an
existing ordinal silently. The per-pilot import/transaction caller still owns
stable identity and external-record fingerprint validation.

A transient `CampaignGenerationInputs.ProvenanceKnown` mask can admit a known
active prefix with unknown inactive physical metadata. Reserved and cached active
reads must be known; source appenders define their new bytes before use. A
successful partial-input preparation publishes only complete80-byte final
provenance. The mask is not serialized as a captured original carrier and does
not make missing old active bytes recoverable.

`ApplyTrainingCacheEffect` is a pure known-file source transaction over a distinct
validated retained Campaign owner snapshot and actual Training launch/result
inputs. It preserves raw framing/header/opaque bytes and applies any nonzero
source credit nibble. Absent files are not created; unknown files reject. This
helper does not widen active-Campaign Training gameplay/result admission: its
full OBJECT preparation epoch remains excluded, so it cannot commit a speculative
Training-derived Campaign draft. Source aliasing is not replaced by an implicit
Campaign-isolation policy.

### N.6 frozen reserve facts and transient scheduling

Fresh five-actor result construction uses frozen context version2 with exactly
four ordered source reserve actor5..8 kind/carrier facts. The finalizer validates
identity and carrier0..4 before applying actual carrier-return80 inheritance.
Context1 keeps its historical interpretation and rejects appended rescue facts;
Single-player contexts remain1. This does not reinterpret old immutable v1/v2
proposals, change canonical pilot/roster schemas or re-award consumed IDs.
Compression request/credit and terminal delivery acknowledgements are transient;
they do not restore an unfinished sortie on startup. N.1 finalack, abandonment,
external conflict checks and independent Phoenix continue unchanged.

## Managed native films

The current storage version is G2R 2. Each film in `AppDir/films` holds an independent level-3 zstd launch frame, operation frames, a checked directory and a completion footer. The launch carries the initial editable description when available. SETUP admits 1–1024 MiB per new recording, default 128 MiB. The budget includes compressed bytes, directory, sealing and publication. An interrupted private prefix can be inspected explicitly. Only a complete verified film is eligible for ordinary publication.

T1 compressed films retain their recorded gameplay and input profiles. Uncompressed storage version 1 is retired. The older Step O and O.1 format documents describe the historical envelope. For compatible compressed films the explicit historical cursor-recovery receipt still binds the film ID, byte digest, content and ending. Practice Resume and playback are isolated from career persistence.

The rolling pointer is `last.json`; names never form paths. A rename preserves the film ID and payload and writes small content-bound metadata. Unscored films use a separate disposition. Recording failure preserves valid career commitment and the prior published Last Mission. Exported missions have their own managed documents.

## Step P authored Simulation and documents

New sortie records/references are schema5 and explicitly pin proposal interpretation3.
Schemas1–4 and proposal versions1/2 retain strict historical validation; schema4
pins2, schema5 pins3. Raw2 authored Simulation is admitted only by interpretation3,
with five actual crew/aircraft and a complete source result context. Its arithmetic
uses the established corrected scoring order; old careers are never rescored.
Pilot/roster schemas remain unchanged. Durable old journals finish their immutable
bytes once; unfinished sessions still abandon. Final acknowledgement, consumed
identities, external fingerprints and independent Phoenix retain N.1 ownership.

AppDir/missions contains independent .g2m schema1 documents (4MiB encoded limit),
with stable random IDs, contained paths, strict typed fields, source dependencies,
ordered descriptors and reproducible compiled realization. Atomic publication
checks the prior file fingerprint; failed validation/compilation/save preserves
the dirty draft and prior complete document. Editing, browsing and Film Export
have no career authority. BEGIN binds the current posted pilot/crew and freezes
a separate accepted launch; it never inherits the exported film's pilot ID.
This is a native format, not original SIM byte compatibility.

New authored films retain container1/input logical-operations-v2 and explicitly
use native-non-ai-stepP-authored-v1. Existing O/O.1 profiles/recovery receipts stay
strict. Exact verified resource pairs admit Preparation26 without ignoring unknown
hashes. The audio handoff repair is transport-only and does not change film ABI.
See evidence/stepP/compatibility.md for precise resource identities and guards.

## Step P.3 — commission choices and immutable historical results

New sortie starts pin proposal interpretation **4** and sortie journal schema **6**.
Canonical pilot/roster schemas are unchanged. Proposal1–3 and journal1–5 retain
their original decisions and arithmetic; durable journals still finish exactly
once. P.3 never rescores an already committed career.

An offered commission now carries separate source CRT continuations for the
player's actual acceptance/refusal. Accepted one-aircraft Single commission
performs source squadron replacement and its separate REPLACE/name page. Refusal
does not generate those four rows. On the eligible multi-aircraft CREWPRO path,
chosen awards/promotions precede replacement initialization. The immutable pending
choice records its verified source continuation; redraws and typing consume no
live simulation or result RNG. Ten-byte optional names remain pending until final
acknowledgement. Conflicts, abandonment, Phoenix and consumed IDs keep their
existing owners.

New films use `native-non-ai-stepP3-v1` or
`native-non-ai-stepP3-authored-v1`. Their ordinary kind21 formation consumes the
source shared LFSR in phase order. Historical film launch/checkpoints retain the
old interpretation, including recorded incomplete successors; they are never
regenerated. Preparation28 adds REPLACE/editor UI metadata only, with explicit
whole-profile mappings to validated older media. Native mission documents retain
old descriptor/realization interpretation; newly selected editor entries identify
the private source-initialized selection view. BEGIN reuses an unchanged saved
realization. See evidence/stepP3/compatibility.md for the delivered profile IDs.


## R2.2 transient wingman damage and film ownership

New ordinary/authored R22 interpretations optionally retain NPC damage masks
5FE/5FF and physical600..609. Checkpoints, isolated Resume, GPS and frozen result
facts use that full12 state. Old film payloads omit this optional owner and keep
their historical interpretation. No personal save/document migration, replay
rescore or abandoned-sortie award occurs. Optional recording failure remains
independent of valid career publication. See
the historical R2.2 compatibility report (research archive, `evidence/stepR22/compatibility.md`).

## R3 transient player copilot ownership

New ordinary/authored simulation profiles are
`native-player-copilot-stepR3-v1` and
`native-player-copilot-stepR3-authored-v1`. They retain persistent physical AEB,
effective defensive state and an optional immutable `Defenses.R3` owner containing
manual/automatic intent, the shared radar token and decoy render-node selector.
They preserve the existing one IR reservoir, cartridge counts, timers, decoy pose
and drift. No owner is added to historical checkpoint JSON. Recorded launch
settings and simulation ticks determine positive58F2 work; current SETUP and
controller state cannot reinterpret playback. Resume remains isolated practice.
No config, controls, mission-document or durable-career schema is migrated.
